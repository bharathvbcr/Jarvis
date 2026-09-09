# External inspiration and boundaries

These references inform design; they are not proof that Jarvis reproduces each project's guarantees.

| Reference | Applied idea |
|---|---|
| [Playwright actionability](https://playwright.dev/docs/actionability) | Unique semantic targets, bounded waits and explicit refusal reasons |
| [Playwright Trace Viewer](https://playwright.dev/docs/trace-viewer) | Before/action/after frames and step navigation |
| [Temporal workflow execution](https://docs.temporal.io/workflow-execution) | Pure transitions and recorded external results, without a Temporal service |
| [OSWorld V2 task classes](https://github.com/xlang-ai/OSWorld-V2/blob/main/evaluation_examples/task_class/README.md) | Independent setup and outcome evaluators unavailable to the agent |
| [Microsoft UFO application states](https://github.com/microsoft/UFO/blob/main/documents/docs/ufo2/app_agent/state.md) | Explicit execution, pending input, confirmation and failure states |
| [egui_kittest](https://github.com/emilk/egui/tree/0.36.2/crates/egui_kittest) | Fast semantic UI regressions, kept separate from native accessibility proof |
| [Perfetto trace formats](https://perfetto.dev/docs/getting-started/other-formats) | Compatible event trace export with existing serializers |
| [SLSA verification guidance](https://slsa.dev/spec/v1.2/verifying-artifacts) | Artifact identity and independently configured expectations are different trust inputs |

Integration uses the projects' documented APIs and platform bindings. No foreign application's controller is copied into the bank. The egui AccessKit SetValue workaround is a shared Rust widget adapter tested through actual AccessKit action events; it is required because egui0.36.2 text fields did not handle that action despite native accessibility advertising it.
