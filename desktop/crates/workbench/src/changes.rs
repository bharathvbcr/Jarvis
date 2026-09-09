//! Differences over sanitized observations only. A traversal path is an
//! observation-local comparison aid, never a persistent capability locator.
use crate::model::{Node, Observation};
use std::collections::BTreeMap;

pub struct Changes {
    pub rows: Vec<String>,
    pub total: usize,
}

pub fn accessibility(before: &Observation, after: &Observation) -> Result<Changes, String> {
    if !before.complete || !after.complete || before.nodes.len() > 2000 || after.nodes.len() > 2000
    {
        return Err("Complete bounded accessibility trees are required for comparison.".into());
    }
    if before.window.pid != after.window.pid || before.window.window_id != after.window.window_id {
        return Err("Observations refer to different application windows.".into());
    }
    fn indexed(nodes: &[Node]) -> Result<BTreeMap<String, &Node>, String> {
        let mut map = BTreeMap::new();
        for node in nodes {
            let key = if node.native_path.is_empty() {
                format!("id:{}", node.id)
            } else {
                format!("path:{:?}", node.native_path)
            };
            if map.insert(key, node).is_some() {
                return Err(
                    "Duplicate accessibility identities prevent an unambiguous comparison.".into(),
                );
            }
        }
        Ok(map)
    }
    let previous = indexed(&before.nodes)?;
    let next = indexed(&after.nodes)?;
    let mut rows = Vec::new();
    let mut total = 0;
    let mut add = |row: String| {
        total += 1;
        if rows.len() < 200 {
            rows.push(row);
        }
    };
    for (key, node) in &previous {
        let Some(current) = next.get(key) else {
            add(format!("Removed {} · {}", node.role, node.name));
            continue;
        };
        let mut changed = Vec::new();
        if node.role != current.role {
            changed.push("role");
        }
        if node.name != current.name {
            changed.push("name");
        }
        if node.value != current.value {
            changed.push("value");
        }
        if node.enabled != current.enabled {
            changed.push("enabled");
        }
        if node.editable != current.editable {
            changed.push("editable");
        }
        if node.identifier != current.identifier {
            changed.push("identifier");
        }
        if node.bounds != current.bounds {
            changed.push("bounds");
        }
        if node.actions != current.actions {
            changed.push("actions");
        }
        if !changed.is_empty() {
            add(format!(
                "{} · {}: {} · value {:?} → {:?}",
                current.role,
                current.name,
                changed.join(", "),
                node.value,
                current.value
            ));
        }
    }
    for (key, node) in &next {
        if !previous.contains_key(key) {
            add(format!("Added {} · {}", node.role, node.name));
        }
    }
    Ok(Changes { rows, total })
}

#[cfg(test)]
mod tests {
    use super::accessibility;
    use crate::model::Observation;
    use serde_json::json;
    fn observation() -> Observation {
        serde_json::from_value(json!({"observation_id":"a","epoch":1,"complete":true,"window":{"pid":1,"window_id":2,"title":"Bank","foreground":true},"nodes":[{"id":"ephemeral-1","native_path":[0,1],"role":"text_field","name":"Status","value":"Ready","enabled":true,"editable":false}],"screenshot":{"mime_type":"image/png","base64":"","width":1,"height":1}})).unwrap()
    }
    #[test]
    fn tracks_real_value_and_geometry_changes_across_ephemeral_ids() {
        let before = observation();
        let mut after = before.clone();
        after.nodes[0].id = "ephemeral-2".into();
        assert_eq!(accessibility(&before, &after).unwrap().total, 0);
        after.nodes[0].value = Some("Member found".into());
        let changes = accessibility(&before, &after).unwrap();
        assert_eq!(changes.total, 1);
        assert!(changes.rows[0].contains("Ready"));
        assert!(changes.rows[0].contains("Member found"));
    }
    #[test]
    fn incomplete_foreign_and_duplicate_identity_trees_do_not_look_unchanged() {
        let before = observation();
        let mut after = before.clone();
        after.complete = false;
        assert!(accessibility(&before, &after).is_err());
        after.complete = true;
        after.window.window_id += 1;
        assert!(accessibility(&before, &after).is_err());
        after = before.clone();
        after.nodes.push(after.nodes[0].clone());
        assert!(accessibility(&before, &after).is_err());
    }
}
