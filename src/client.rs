use crate::{
    Result,
    body::extension,
    config::{Config, Endpoint, escape},
};
use base64::{Engine, engine::general_purpose::STANDARD};
use reqwest::{Method, blocking::Client};
use serde_json::{Map, Value};
use std::{
    fs::{self, OpenOptions},
    io::{Read, Write},
    path::{Path, PathBuf},
    time::{Duration, Instant},
};

pub fn create() -> Result<Client> {
    let _ = rustls::crypto::ring::default_provider().install_default();
    Ok(Client::builder()
        .user_agent(concat!("pxlb/", env!("PXLB_VERSION")))
        .retry(reqwest::retry::never())
        .timeout(None)
        .connect_timeout(Duration::from_secs(30))
        .build()?)
}
fn go_value(value: &Value) -> String {
    match value {
        Value::String(value) => value.clone(),
        Value::Null => "<nil>".into(),
        Value::Array(values) => format!(
            "[{}]",
            values.iter().map(go_value).collect::<Vec<_>>().join(" ")
        ),
        Value::Object(values) => format!(
            "map[{}]",
            values
                .iter()
                .map(|(key, value)| format!("{key}:{}", go_value(value)))
                .collect::<Vec<_>>()
                .join(" ")
        ),
        other => other.to_string(),
    }
}

pub fn execute(
    client: &Client,
    config: &Config,
    spec: &Endpoint,
    path: &str,
    body: Option<&Map<String, Value>>,
    remaining: Option<Duration>,
) -> Result<(Vec<u8>, String)> {
    let url = format!("{}{path}", config.base.trim_end_matches('/'));
    let mut request = client.request(Method::from_bytes(spec.method.as_bytes())?, url);
    let query: Vec<_> = spec
        .query
        .iter()
        .filter_map(|name| {
            config
                .flags
                .get(&name.replace('_', "-"))
                .and_then(|values| values.last())
                .map(|value| (name, value))
        })
        .collect();
    request = request.query(&query);
    if let Some(body) = body {
        request = request
            .header("Content-Type", "application/json")
            .body(serde_json::to_vec(body)?);
    }
    if !config.token.is_empty() {
        request = request.bearer_auth(&config.token);
    }
    if let Some(remaining) = remaining {
        request = request.timeout(remaining);
    }
    let started = Instant::now();
    let mut response = match request.send() {
        Ok(response) => response,
        Err(error) if error.is_timeout() => return Err("request timeout".into()),
        Err(error) => return Err(error.without_url().into()),
    };
    let status = response.status();
    let content_type = response
        .headers()
        .get("Content-Type")
        .and_then(|value| value.to_str().ok())
        .unwrap_or("")
        .to_owned();
    let mut bytes = Vec::new();
    if let Err(error) = std::io::Read::take(&mut response, (512 << 20) + 1).read_to_end(&mut bytes)
    {
        // The blocking response wraps body timeouts in io::Error; unlike send(),
        // its Display text does not reliably include the underlying timeout.
        if error.kind() == std::io::ErrorKind::TimedOut
            || error
                .get_ref()
                .and_then(|source| source.downcast_ref::<reqwest::Error>())
                .is_some_and(reqwest::Error::is_timeout)
            || remaining.is_some_and(|budget| started.elapsed() >= budget)
        {
            return Err("request timeout".into());
        }
        return Err(error.into());
    }
    if bytes.len() > 512 << 20 {
        return Err("API response exceeds 512 MiB".into());
    }
    if !status.is_success() {
        let text = String::from_utf8_lossy(&bytes);
        let mut message = text.trim().to_owned();
        if let Ok(value) = serde_json::from_slice::<Value>(&bytes)
            && let Some(detail) = value.get("detail").or_else(|| value.get("error"))
        {
            message = go_value(detail);
        }
        if message.is_empty() {
            message = status.canonical_reason().unwrap_or("").into();
        }
        return Err(format!("API error {}: {message}", status.as_u16()).into());
    }
    Ok((bytes, content_type))
}

pub fn job_id(value: &Value) -> Option<&str> {
    match value {
        Value::Object(values) => {
            for key in ["background_job_id", "job_id"] {
                if let Some(id) = values
                    .get(key)
                    .and_then(Value::as_str)
                    .filter(|id| !id.is_empty())
                {
                    return Some(id);
                }
            }
            if values
                .get("status")
                .map(go_value)
                .unwrap_or_default()
                .to_ascii_lowercase()
                .contains("process")
                && let Some(id) = values.get("id").and_then(Value::as_str)
            {
                return Some(id);
            }
            values.values().find_map(job_id)
        }
        Value::Array(values) => values.iter().find_map(job_id),
        _ => None,
    }
}

pub fn wait(client: &Client, config: &Config, initial: Value) -> Result<Value> {
    let Some(id) = job_id(&initial) else {
        return Ok(initial);
    };
    let id = id.to_owned();
    let deadline = if config.timeout.is_zero() {
        None
    } else {
        Some(
            Instant::now()
                .checked_add(config.timeout)
                .ok_or("job timeout is too large")?,
        )
    };
    let spec = Endpoint {
        method: "GET".into(),
        path: String::new(),
        required: Vec::new(),
        query: Vec::new(),
        description: String::new(),
    };
    loop {
        let remaining = if let Some(deadline) = deadline {
            Some(
                deadline
                    .checked_duration_since(Instant::now())
                    .ok_or_else(|| format!("timeout waiting for job {id}"))?,
            )
        } else {
            None
        };
        let (bytes, _) = execute(
            client,
            config,
            &spec,
            &format!("/background-jobs/{}", escape(&id)),
            None,
            remaining,
        )?;
        let payload: Value = serde_json::from_slice(&bytes)?;
        if !payload.is_object() {
            return Err("job status is not a JSON object".into());
        }
        match payload["status"]
            .as_str()
            .unwrap_or("")
            .to_ascii_lowercase()
            .as_str()
        {
            "completed" | "complete" | "succeeded" | "success" => {
                return Ok(if payload["last_response"].is_object() {
                    payload["last_response"].clone()
                } else {
                    payload
                });
            }
            "failed" | "error" => {
                return Err(format!("job {id} failed: {}", go_value(&payload)).into());
            }
            _ => {}
        }
        let delay = if let Some(deadline) = deadline {
            config
                .poll
                .min(deadline.saturating_duration_since(Instant::now()))
        } else {
            config.poll
        };
        std::thread::sleep(delay);
    }
}

fn directory(path: &Path) -> Result<()> {
    if path.as_os_str().is_empty() {
        return Ok(());
    }
    let mut builder = fs::DirBuilder::new();
    builder.recursive(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::DirBuilderExt;
        builder.mode(0o755);
    }
    builder.create(path)?;
    Ok(())
}
pub fn write_bytes(path: &Path, bytes: &[u8]) -> Result<()> {
    let path = if extension(path).is_empty() {
        directory(path)?;
        path.join("response.bin")
    } else {
        if let Some(parent) = path.parent() {
            directory(parent)?;
        }
        path.to_owned()
    };
    let mut options = OpenOptions::new();
    options.write(true).create(true).truncate(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o600);
    }
    let mut file = options.open(path)?;
    file.write_all(bytes)?;
    file.flush()?;
    Ok(())
}
fn images<'a>(payload: &'a Value, found: &mut Vec<&'a Map<String, Value>>) {
    match payload {
        Value::Object(values) => {
            if values
                .get("base64")
                .and_then(Value::as_str)
                .is_some_and(|value| !value.is_empty())
            {
                found.push(values);
            }
            for value in values.values() {
                images(value, found);
            }
        }
        Value::Array(values) => {
            for value in values {
                images(value, found);
            }
        }
        _ => {}
    }
}
pub fn write_images(out: &str, payload: &Value) -> Result<()> {
    let mut found = Vec::new();
    images(payload, &mut found);
    if found.is_empty() {
        return Ok(());
    }
    let single = found.len() == 1 && !extension(Path::new(out)).is_empty();
    if !single {
        directory(Path::new(out))?;
    }
    for (index, image) in found.iter().enumerate() {
        let mut raw = image["base64"].as_str().unwrap_or("");
        if let Some((prefix, data)) = raw.split_once(',')
            && prefix.contains("base64")
        {
            raw = data;
        }
        let bytes = STANDARD.decode(raw.replace(['\r', '\n'], ""))?;
        let path = if single {
            PathBuf::from(out)
        } else {
            let format = image
                .get("format")
                .map(go_value)
                .filter(|value| !value.is_empty() && value != "<nil>")
                .unwrap_or_else(|| "png".into());
            let format = format.strip_prefix('.').unwrap_or(&format);
            if format.is_empty()
                || format.len() > 16
                || !format.bytes().all(|byte| byte.is_ascii_alphanumeric())
            {
                return Err("API returned an invalid image format".into());
            }
            Path::new(out).join(format!("image_{index:02}.{format}"))
        };
        write_bytes(&path, &bytes)?;
    }
    Ok(())
}
