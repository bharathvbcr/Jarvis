use jarvis_bank::{
    BankApp, Fault,
    state::{BankStore, Tenant},
};
use std::path::PathBuf;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args == ["--help"] {
        println!(
            "jarvis-bank --tenant north|south --state PATH [--fault none|overlay|delay|missing-control|duplicate-control|commit-noop|false-ack|crash-after-commit]"
        );
        return Ok(());
    }
    let mut tenant = None;
    let mut state = None;
    let mut fault = None;
    for pair in args.chunks(2) {
        if pair.len() != 2 {
            return Err("Every flag requires a value; use --help".into());
        }
        match pair[0].as_str() {
            "--tenant" if tenant.is_none() => tenant = Some(Tenant::parse(&pair[1])?),
            "--state" if state.is_none() => state = Some(PathBuf::from(&pair[1])),
            "--fault" if fault.is_none() => fault = Some(Fault::parse(&pair[1])?),
            _ => return Err("Unknown or repeated flag; use --help".into()),
        }
    }
    let tenant = tenant.ok_or("--tenant is required")?;
    let state = state.ok_or("--state is required")?;
    let store = BankStore::open(&state, tenant)?;
    let title = format!("Jarvis Bank — {}", tenant.title());
    let options = eframe::NativeOptions {
        viewport: eframe::egui::ViewportBuilder::default()
            .with_title(&title)
            .with_inner_size([880.0, 700.0])
            .with_min_inner_size([760.0, 640.0]),
        ..Default::default()
    };
    eframe::run_native(
        &title,
        options,
        Box::new(move |cc| {
            cc.egui_ctx.set_visuals(eframe::egui::Visuals::light());
            Ok(Box::new(BankApp::new(store, fault.unwrap_or(Fault::None))))
        }),
    )?;
    Ok(())
}
