mod body;
mod client;
mod config;
use serde_json::Value;
use std::{
    io::{self, Write},
    path::Path,
};
type Result<T> = std::result::Result<T, Box<dyn std::error::Error + Send + Sync>>;

fn run(args: &[String]) -> i32 {
    let Some(route) = args.first() else {
        eprintln!("usage: pxlb <api-route> [flags]");
        return 2;
    };
    if matches!(route.as_str(), "--version" | "-v" | "version") {
        println!(
            "pxlb {}\ncommit: {}\nbuilt: {}",
            env!("PXLB_VERSION"),
            env!("PXLB_COMMIT"),
            env!("PXLB_DATE")
        );
        return 0;
    }
    if matches!(route.as_str(), "--help" | "-h" | "help") {
        print!("{}", config::help());
        return 0;
    }
    let prepared = (|| {
        let config = config::Config::parse(&args[1..])?;
        let (spec, path) = config::resolve(route, &config)?;
        let body = body::build(&spec, &config)?;
        Ok::<_, Box<dyn std::error::Error + Send + Sync>>((config, spec, path, body))
    })();
    let (config, spec, path, body) = match prepared {
        Ok(value) => value,
        Err(error) => {
            eprintln!("{error}");
            return 2;
        }
    };
    let result = (|| {
        let client = client::create()?;
        let (bytes, media) = client::execute(&client, &config, &spec, &path, body.as_ref(), None)?;
        if !config.out.is_empty()
            && !media.is_empty()
            && !media.to_ascii_lowercase().contains("json")
        {
            client::write_bytes(Path::new(&config.out), &bytes)?;
            return Ok(());
        }
        let value = serde_json::from_slice::<Value>(&bytes)
            .ok()
            .filter(|value| value.is_object() || value.is_null());
        let Some(mut value) = value else {
            if !config.out.is_empty() {
                client::write_bytes(Path::new(&config.out), &bytes)?;
            } else {
                let mut stdout = io::stdout().lock();
                stdout.write_all(&bytes)?;
                writeln!(stdout)?;
            }
            return Ok(());
        };
        if config.wait {
            value = client::wait(&client, &config, value)?;
        }
        if !config.out.is_empty() {
            client::write_images(&config.out, &value)?;
        }
        if !config.quiet {
            let mut stdout = io::stdout().lock();
            serde_json::to_writer_pretty(&mut stdout, &value)?;
            writeln!(stdout)?;
        }
        Ok::<_, Box<dyn std::error::Error + Send + Sync>>(())
    })();
    match result {
        Ok(()) => 0,
        Err(error) => {
            eprintln!("{error}");
            1
        }
    }
}
fn main() {
    std::process::exit(run(&std::env::args().skip(1).collect::<Vec<_>>()));
}
