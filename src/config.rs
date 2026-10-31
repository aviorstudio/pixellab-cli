use crate::Result;
use serde::Deserialize;
use std::{collections::BTreeMap, fs, time::Duration};

#[derive(Clone, Deserialize)]
pub struct Endpoint {
    pub method: String,
    pub path: String,
    pub required: Vec<String>,
    pub query: Vec<String>,
    pub description: String,
}
pub fn endpoints() -> Vec<Endpoint> {
    serde_json::from_str(include_str!("endpoints.json")).expect("validated endpoint registry")
}
pub const BASE: &str = "https://api.pixellab.ai/v2";
pub struct Config {
    pub token: String,
    pub base: String,
    pub method: String,
    pub out: String,
    pub wait: bool,
    pub poll: Duration,
    pub timeout: Duration,
    pub body: String,
    pub quiet: bool,
    pub flags: BTreeMap<String, Vec<String>>,
}
pub fn boolean(value: &str) -> bool {
    !matches!(value.to_ascii_lowercase().as_str(), "false" | "f" | "0")
}
fn duration(value: &str) -> Result<Duration> {
    if value == "0" {
        return Ok(Duration::ZERO);
    }
    if value.is_empty() {
        return Err("empty duration".into());
    }
    let negative = value.starts_with('-');
    let mut rest = value.trim_start_matches(['-', '+']);
    let mut seconds = 0.;
    if rest.is_empty() {
        return Err("invalid duration".into());
    }
    while !rest.is_empty() {
        let count = rest
            .find(|c: char| !c.is_ascii_digit() && c != '.')
            .ok_or("duration needs a unit")?;
        let amount: f64 = rest[..count].parse()?;
        rest = &rest[count..];
        let (unit, scale) = [
            ("ns", 1e-9),
            ("us", 1e-6),
            ("µs", 1e-6),
            ("μs", 1e-6),
            ("ms", 1e-3),
            ("s", 1.),
            ("m", 60.),
            ("h", 3600.),
        ]
        .into_iter()
        .find(|(unit, _)| rest.starts_with(unit))
        .ok_or("invalid duration unit")?;
        seconds += amount * scale;
        rest = &rest[unit.len()..];
    }
    if !seconds.is_finite() || seconds > i64::MAX as f64 / 1e9 {
        return Err("duration exceeds supported range".into());
    }
    Ok(if negative {
        Duration::ZERO
    } else {
        Duration::try_from_secs_f64(seconds)?
    })
}
pub fn token() -> String {
    if let Ok(token) = std::env::var("PIXELLAB_API_KEY")
        && !token.is_empty()
    {
        return token;
    }
    let Ok(text) = fs::read_to_string(".env") else {
        return String::new();
    };
    let mut token = String::new();
    for line in text.lines() {
        let line = line.trim();
        if line.starts_with('#') {
            continue;
        }
        let line = line.strip_prefix("export ").unwrap_or(line);
        if let Some((key, value)) = line.split_once('=')
            && key.trim() == "PIXELLAB_API_KEY"
        {
            token = value.trim().trim_matches(['\"', '\'']).into();
        }
    }
    token
}
impl Config {
    pub fn parse(args: &[String]) -> Result<Self> {
        let mut config = Self {
            token: token(),
            base: BASE.into(),
            method: String::new(),
            out: String::new(),
            wait: false,
            poll: Duration::from_secs(5),
            timeout: Duration::ZERO,
            body: String::new(),
            quiet: false,
            flags: BTreeMap::new(),
        };
        let mut index = 0;
        while index < args.len() {
            let arg = &args[index];
            index += 1;
            let Some(name) = arg.strip_prefix("--") else {
                return Err(format!("unexpected positional argument: {arg}").into());
            };
            let value = if args
                .get(index)
                .is_some_and(|value| !value.starts_with("--"))
            {
                let value = args[index].clone();
                index += 1;
                value
            } else {
                "true".into()
            };
            match name {
                "token" => config.token = value,
                "base-url" => config.base = value,
                "http-method" => {
                    config.method = value.to_ascii_uppercase();
                    if !matches!(config.method.as_str(), "GET" | "POST" | "PATCH" | "DELETE") {
                        return Err(format!("invalid --http-method: {value}").into());
                    }
                }
                "json" => {}
                "out" => config.out = value,
                "wait" => config.wait = boolean(&value),
                "poll-interval" => config.poll = duration(&value)?,
                "timeout" => config.timeout = duration(&value)?,
                "body-json" => config.body = value,
                "quiet" => config.quiet = boolean(&value),
                _ => config.flags.entry(name.into()).or_default().push(value),
            }
        }
        Ok(config)
    }
}
pub fn placeholder(value: &str) -> bool {
    (value.starts_with('{') && value.ends_with('}')
        || value.starts_with('<') && value.ends_with('>'))
        && value.len() > 2
}
pub fn escape(value: &str) -> String {
    let mut out = String::new();
    for byte in value.bytes() {
        if byte.is_ascii_alphanumeric() || b"-_.~$&+:=@".contains(&byte) {
            out.push(byte as char);
        } else {
            out.push_str(&format!("%{byte:02X}"));
        }
    }
    out
}
pub fn resolve(route: &str, config: &Config) -> Result<(Endpoint, String)> {
    let route = format!("/{}", route.trim_start_matches('/'));
    let path: Vec<_> = route.trim_matches('/').split('/').collect();
    let mut matches = Vec::new();
    for spec in endpoints() {
        let template: Vec<_> = spec.path.trim_matches('/').split('/').collect();
        if template.len() != path.len() {
            continue;
        }
        let mut parts = Vec::new();
        let mut valid = true;
        for (template, actual) in template.iter().zip(&path) {
            if placeholder(actual) || actual.is_empty() {
                valid = false;
                break;
            }
            if template.starts_with('{') {
                parts.push(escape(actual));
            } else if template == actual {
                parts.push(template.to_string());
            } else {
                valid = false;
                break;
            }
        }
        if valid {
            matches.push((spec, format!("/{}", parts.join("/"))));
        }
    }
    if matches.is_empty() {
        return Err(format!("unknown endpoint: {route}").into());
    }
    let available = matches
        .iter()
        .map(|(spec, _)| spec.method.to_ascii_lowercase())
        .collect::<Vec<_>>()
        .join(", ");
    if !config.method.is_empty() {
        return matches.into_iter().find(|(spec,_)|spec.method==config.method).ok_or_else(||format!("endpoint {route} does not support --http-method {}; available methods: {available}",config.method.to_ascii_lowercase()).into());
    }
    if matches.len() == 1 {
        return Ok(matches.remove(0));
    }
    let query: Vec<_> = matches
        .iter()
        .flat_map(|(spec, _)| spec.query.iter().map(|v| v.replace('_', "-")))
        .collect();
    if !config.body.is_empty() || config.flags.keys().any(|key| !query.contains(key)) {
        let count = matches
            .iter()
            .filter(|(spec, _)| !matches!(spec.method.as_str(), "GET" | "DELETE"))
            .count();
        if count == 1 {
            return Ok(matches
                .into_iter()
                .find(|(spec, _)| !matches!(spec.method.as_str(), "GET" | "DELETE"))
                .unwrap());
        }
    }
    matches
        .into_iter()
        .find(|(spec, _)| spec.method == "GET")
        .ok_or_else(|| {
            format!("ambiguous endpoint: {route}; use --http-method with one of: {available}")
                .into()
        })
}
pub fn display(path: &str) -> String {
    path.trim_start_matches('/')
        .split('/')
        .map(|part| {
            if placeholder(part) {
                format!("<{}>", &part[1..part.len() - 1])
            } else {
                part.into()
            }
        })
        .collect::<Vec<_>>()
        .join("/")
}
pub fn help() -> String {
    let mut text="pxlb - PixelLab v2 API CLI\n\nUsage:\n  pxlb <api-route> [flags]\n  pxlb --version\n\nGlobal flags:\n  --token <key>            PixelLab API key; overrides PIXELLAB_API_KEY and .env.\n  --base-url <url>         API base URL; defaults to https://api.pixellab.ai/v2.\n  --http-method <method>   Override inferred HTTP method for colliding routes.\n  --body-json <json|file>  Send a raw JSON request body.\n  --wait                   Poll returned background jobs until completion.\n  --poll-interval <time>   Job polling interval, default 5s.\n  --timeout <time>         Optional job polling timeout.\n  --out <path>             Save returned binary or base64 image outputs.\n  --quiet                  Suppress normal JSON output.\n\nRoutes:\n".to_owned();
    let mut entries: Vec<_> = endpoints()
        .into_iter()
        .map(|spec| {
            let command = if spec.method == "DELETE" {
                format!("{} --http-method delete", display(&spec.path))
            } else if spec.path == "/tilesets" && spec.method == "POST" {
                format!(
                    "{} --lower-description ... --upper-description ...",
                    display(&spec.path)
                )
            } else {
                display(&spec.path)
            };
            (command, spec.description)
        })
        .collect();
    entries.sort();
    let width = entries
        .iter()
        .map(|(command, _)| command.len())
        .max()
        .unwrap_or(0);
    for (command, description) in entries {
        text.push_str(&format!("  {command:width$}  {description}\n"));
    }
    text
}
