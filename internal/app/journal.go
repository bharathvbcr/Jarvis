package app

import (
	"encoding/base64"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bharathvbcr/Jarvis/internal/evidence"
	"github.com/bharathvbcr/Manvi/manvi/computer"
)

// desktopJournal is the shared durable sink for discovery and frozen replay.
// Callers serialize access and supply only privacy-approved observations.
type desktopJournal struct {
	dir          string
	file         *os.File
	records      int
	imageBytes   int64
	journalBytes int64
	// sync is the only flush the journal issues: frame files, tree files and the
	// event line all go through it. It is (*os.File).Sync (F_FULLFSYNC on
	// darwin) except where a test counts the flushes.
	sync func(*os.File) error
}

func openDesktopJournal(dir string) (*desktopJournal, error) {
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &desktopJournal{dir: dir, file: f, sync: (*os.File).Sync}, nil
}

// encodeLine is the bytes of one events.jsonl line (without the newline) and
// encodeTree the bytes of one frame-NNNNNN.json file. They are the journal's
// wire contract, pinned by TestJournalEncodingIsStable.
//
// Both call encoding/json/v2 with the v1 option set, so the bytes are exactly
// what encoding/json v1 wrote (HTML escaping, null for nil slices, v1 omitempty
// and no zero-value fields added). Dropping an option changes the evidence
// format: v2's own defaults add "strategy_index":0, "pointer_motion":false and
// similar zero fields, write [] for nil slices, and stop escaping < > &.
func encodeLine(record computer.Record) ([]byte, error) {
	return jsonv2.Marshal(record, json.DefaultOptionsV1())
}
func encodeTree(o computer.Observation) ([]byte, error) {
	return jsonv2.Marshal(o, json.DefaultOptionsV1(), jsontext.WithIndent("  "))
}
func (j *desktopJournal) append(record computer.Record, frameSequence uint64) (computer.Record, []evidence.Artifact, error) {
	j.records++
	if j.records > 4096 {
		return record, nil, errors.New("run evidence record limit exceeded")
	}
	artifacts := []evidence.Artifact{}
	if record.Kind == "observation" && record.Observation != nil {
		o := *record.Observation
		imageBytes, err := base64.StdEncoding.DecodeString(o.Screenshot.Base64)
		if err != nil {
			return record, nil, err
		}
		j.imageBytes += int64(len(imageBytes))
		if j.imageBytes > 256<<20 {
			return record, nil, errors.New("run evidence exceeds 256 MiB")
		}
		prefix := fmt.Sprintf("frame-%06d", frameSequence)
		if err = writeAtomicSync(filepath.Join(j.dir, prefix+".png"), imageBytes, j.sync); err != nil {
			return record, nil, err
		}
		artifacts = append(artifacts, evidence.Artifact{ID: prefix, Path: prefix + ".png", SHA256: digest(imageBytes), SizeBytes: uint64(len(imageBytes))})
		o.Screenshot.Base64 = ""
		treeBytes, err := encodeTree(o)
		if err != nil {
			return record, nil, err
		}
		if err = writeAtomicSync(filepath.Join(j.dir, prefix+".json"), treeBytes, j.sync); err != nil {
			return record, nil, err
		}
		artifacts = append(artifacts, evidence.Artifact{ID: prefix + "-tree", Path: prefix + ".json", SHA256: digest(treeBytes), SizeBytes: uint64(len(treeBytes))})
		record.Observation = &o
	}
	data, err := encodeLine(record)
	if err != nil {
		return record, nil, err
	}
	if len(data) > 1<<20 {
		return record, nil, errors.New("journal record exceeds one MiB")
	}
	j.journalBytes += int64(len(data) + 1)
	if j.journalBytes > 64<<20 {
		return record, nil, errors.New("journal exceeds 64 MiB")
	}
	if _, err = j.file.Write(append(data, '\n')); err != nil {
		return record, nil, err
	}
	return record, artifacts, j.sync(j.file)
}
