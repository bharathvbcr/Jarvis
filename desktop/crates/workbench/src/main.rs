use jarvis_workbench::{app::Workbench, transport::Backend};
use std::path::{Path, PathBuf};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args == ["--help"] {
        println!(
            "jarvis-workbench [--backend PATH] [--root PATH]\nPackaged build locations infer these paths; other locations require explicit paths."
        );
        return Ok(());
    }
    let (backend, root) = resolve_paths(&args, &std::env::current_exe()?)?;
    let transport = Backend::spawn(&backend, &root)?;
    let options = eframe::NativeOptions {
        viewport: eframe::egui::ViewportBuilder::default()
            .with_title("Jarvis Workbench")
            .with_inner_size([1280.0, 900.0])
            .with_min_inner_size([960.0, 720.0]),
        ..Default::default()
    };
    let result = eframe::run_native(
        "Jarvis Workbench",
        options,
        Box::new(move |context| {
            context.egui_ctx.set_visuals(eframe::egui::Visuals::light());
            Ok(Box::new(Workbench::new(transport, backend, root)))
        }),
    );
    jarvis_workbench::transport::finish_shutdown()?;
    result?;
    Ok(())
}

fn resolve_paths(
    args: &[String],
    executable: &Path,
) -> Result<(PathBuf, PathBuf), Box<dyn std::error::Error>> {
    let mut backend: Option<PathBuf> = None;
    let mut root: Option<PathBuf> = None;
    for pair in args.chunks(2) {
        if pair.len() != 2 {
            return Err("Every flag requires a value; use --help".into());
        }
        match pair[0].as_str() {
            "--backend" if backend.is_none() => {
                backend = Some(PathBuf::from(&pair[1]).canonicalize()?)
            }
            "--root" if root.is_none() => root = Some(PathBuf::from(&pair[1]).canonicalize()?),
            _ => return Err("Unknown or repeated flag; use --help".into()),
        }
    }
    let root = match root {
        Some(root) => root,
        None => {
            let inferred = match &backend {
                Some(backend) => backend
                    .parent()
                    .filter(|dir| dir.file_name().is_some_and(|name| name == "build"))
                    .and_then(Path::parent),
                None => package_root(executable),
            };
            inferred
                .ok_or(
                    "Cannot infer product root from executable location; pass --root and --backend",
                )?
                .canonicalize()?
        }
    };
    let backend = match backend {
        Some(backend) => backend,
        None => root
            .join("build")
            .join(format!("jarvis{}", std::env::consts::EXE_SUFFIX))
            .canonicalize()?,
    };
    if !backend.is_file() || !root.is_dir() {
        return Err("Backend must be a file and root must be a directory".into());
    }
    Ok((backend, root))
}

fn package_root(executable: &Path) -> Option<&Path> {
    let name = executable.file_name()?.to_str()?;
    if name != format!("jarvis-workbench{}", std::env::consts::EXE_SUFFIX) {
        return None;
    }
    let parent = executable.parent()?;
    if parent.file_name()? == "build" {
        return parent.parent();
    }
    if parent.file_name()? != "MacOS" {
        return None;
    }
    let contents = parent.parent()?;
    let app = contents.parent()?;
    let build = app.parent()?;
    if contents.file_name()? == "Contents"
        && app.file_name()? == "JarvisWorkbench.app"
        && build.file_name()? == "build"
    {
        return build.parent();
    }
    None
}

#[cfg(test)]
mod tests {
    use super::{package_root, resolve_paths};
    use std::path::{Path, PathBuf};

    #[test]
    fn packaged_layouts_resolve_without_current_directory() {
        let root = Path::new("/submission");
        let plain = root
            .join("build")
            .join(format!("jarvis-workbench{}", std::env::consts::EXE_SUFFIX));
        let app = root
            .join("build/JarvisWorkbench.app/Contents/MacOS")
            .join(format!("jarvis-workbench{}", std::env::consts::EXE_SUFFIX));
        assert_eq!(package_root(&plain), Some(root));
        assert_eq!(package_root(&app), Some(root));
        for ambiguous in [
            "/tmp/jarvis-workbench",
            "/Applications/JarvisWorkbench.app/Contents/MacOS/jarvis-workbench",
            "/submission/build/unrelated",
        ] {
            assert_eq!(package_root(Path::new(ambiguous)), None);
        }
    }

    #[test]
    fn explicit_paths_override_ambiguous_executable_location()
    -> Result<(), Box<dyn std::error::Error>> {
        let executable = std::env::current_exe()?.canonicalize()?;
        let root = executable
            .parent()
            .ok_or("test executable has no directory")?
            .to_path_buf();
        let args = vec![
            "--backend".into(),
            executable.to_string_lossy().into_owned(),
            "--root".into(),
            root.to_string_lossy().into_owned(),
        ];
        assert_eq!(
            resolve_paths(&args, Path::new("/ambiguous/app"))?,
            (executable, root)
        );
        assert!(resolve_paths(&[], &PathBuf::from("/ambiguous/app")).is_err());
        assert!(resolve_paths(&["--root".into()], &PathBuf::from("/ambiguous/app")).is_err());
        Ok(())
    }
}
