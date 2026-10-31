use std::{env, process::Command};
fn main() {
    for name in ["VERSION", "COMMIT", "DATE"] {
        println!("cargo:rerun-if-env-changed={name}");
    }
    let commit = env::var("COMMIT")
        .ok()
        .or_else(|| {
            let output = Command::new("git")
                .args(["rev-parse", "HEAD"])
                .output()
                .ok()?;
            output
                .status
                .success()
                .then(|| String::from_utf8_lossy(&output.stdout).trim().into())
        })
        .unwrap_or_else(|| "none".into());
    for (name, value) in [
        (
            "PXLB_VERSION",
            env::var("VERSION").unwrap_or_else(|_| "dev".into()),
        ),
        ("PXLB_COMMIT", commit),
        (
            "PXLB_DATE",
            env::var("DATE").unwrap_or_else(|_| "unknown".into()),
        ),
    ] {
        assert!(!value.contains(['\r', '\n']));
        println!("cargo:rustc-env={name}={value}");
    }
    println!("cargo:rerun-if-changed=.git");
}
