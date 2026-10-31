use crate::{
    Result,
    config::{Config, Endpoint, display},
};
use base64::{Engine, engine::general_purpose::STANDARD};
use serde_json::{Map, Value, json};
use std::{fs, path::Path};

pub fn infer(value: &str) -> Value {
    match value {
        "true" => json!(true),
        "false" => json!(false),
        _ => {
            if let Ok(number) = value.parse::<i64>() {
                json!(number)
            } else if value.contains('.')
                && let Ok(number) = value.parse::<f64>()
                && number.is_finite()
            {
                json!(number)
            } else {
                json!(value)
            }
        }
    }
}
pub fn maybe_file(value: &str) -> String {
    if value.trim_start().starts_with(['{', '[']) {
        value.into()
    } else {
        fs::read_to_string(value).unwrap_or_else(|_| value.into())
    }
}
pub fn extension(path: &Path) -> String {
    path.file_name()
        .map(|name| {
            let name = name.to_string_lossy();
            name.rfind('.')
                .map(|index| name[index..].to_owned())
                .unwrap_or_default()
        })
        .unwrap_or_default()
}
fn image(path: &str) -> Result<Value> {
    let bytes = fs::read(path)?;
    let format = extension(Path::new(path))
        .trim_start_matches('.')
        .to_ascii_lowercase();
    let format = match format.as_str() {
        "jpg" => "jpeg",
        "" => "png",
        other => other,
    };
    Ok(json!({"type":"base64","base64":STANDARD.encode(bytes),"format":format}))
}
fn size(value: &str) -> Result<Value> {
    if value.contains('x') {
        let parts: Vec<_> = value.split('x').collect();
        if parts.len() != 2 {
            return Err("expected WIDTHxHEIGHT".into());
        }
        Ok(json!({"width":parts[0].parse::<i64>()?,"height":parts[1].parse::<i64>()?}))
    } else {
        Ok(infer(value))
    }
}
pub fn build(spec: &Endpoint, config: &Config) -> Result<Option<Map<String, Value>>> {
    for part in spec.path.split('/').filter(|part| part.starts_with('{')) {
        let name = part.trim_matches(['{', '}']).replace('_', "-");
        if config.flags.contains_key(&name) {
            return Err(format!(
                "route parameter {name} belongs in the route, for example: {}",
                display(&spec.path)
            )
            .into());
        }
    }
    if !config.body.is_empty() {
        let value = serde_json::from_str::<Value>(&maybe_file(&config.body))?;
        return match value {
            Value::Object(value) => Ok(Some(value)),
            Value::Null => Ok(None),
            _ => Err("invalid --body-json: expected an object".into()),
        };
    }
    if matches!(spec.method.as_str(), "GET" | "DELETE") {
        return Ok(None);
    }
    let mut body = Map::new();
    for (flag, values) in &config.flags {
        if spec.query.iter().any(|key| key.replace('_', "-") == *flag)
            || spec
                .path
                .contains(&format!("{{{}}}", flag.replace('-', "_")))
        {
            continue;
        }
        let value = values.last().map(String::as_str).unwrap_or("");
        let (key, value) = match flag.as_str() {
            "tag" => ("tags".into(), json!(values)),
            "index" => (
                "indices".into(),
                json!(
                    values
                        .iter()
                        .map(|value| value.parse::<i64>())
                        .collect::<std::result::Result<Vec<_>, _>>()?
                ),
            ),
            "item-description" => ("item_descriptions".into(), json!(values)),
            "directions" => (
                "directions".into(),
                json!(
                    value
                        .split(',')
                        .map(str::trim)
                        .filter(|s| !s.is_empty())
                        .collect::<Vec<_>>()
                ),
            ),
            "style-image"
                if matches!(
                    spec.path.as_str(),
                    "/generate-with-style-v2" | "/create-1-direction-object"
                ) =>
            {
                (
                    "style_images".into(),
                    Value::Array(
                        values
                            .iter()
                            .map(|value| image(value))
                            .collect::<Result<Vec<_>>>()?,
                    ),
                )
            }
            _ => {
                let key = flag.replace('-', "_");
                if let Some(name) = flag.strip_suffix("-json") {
                    (
                        name.replace('-', "_"),
                        serde_json::from_str(&maybe_file(value))?,
                    )
                } else if flag == "image"
                    || flag.ends_with("-image")
                    || flag.ends_with("-images")
                    || flag.ends_with("-frame")
                {
                    (key, image(value)?)
                } else if values.len() > 1
                    || matches!(flag.as_str(), "style-image" | "reference-image")
                {
                    (
                        key,
                        Value::Array(values.iter().map(|value| infer(value)).collect()),
                    )
                } else if flag == "size" || flag.ends_with("-size") {
                    (key, size(value)?)
                } else {
                    (key, infer(value))
                }
            }
        };
        body.insert(key, value);
    }
    for required in &spec.required {
        if body.get(required).is_none_or(Value::is_null) {
            return Err(format!("missing --{}", required.replace('_', "-")).into());
        }
    }
    Ok(Some(body))
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn scalar_and_size_inference_preserve_existing_wire_types() {
        assert_eq!(infer("1e3"), json!("1e3"));
        assert_eq!(infer("1.5"), json!(1.5));
        assert_eq!(infer("false"), json!(false));
        assert_eq!(size("64x32").unwrap(), json!({"width":64,"height":32}));
        assert_eq!(extension(Path::new(".hidden")), ".hidden");
    }
}
