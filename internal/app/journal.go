package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/devcouncil"
)

// desktopJournal is the shared durable sink for discovery and frozen replay.
// Callers serialize access and supply only privacy-approved observations.
type desktopJournal struct {
	dir          string
	file         *os.File
	records      int
	imageBytes   int64
	journalBytes int64
}

func openDesktopJournal(dir string) (*desktopJournal, error) {
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &desktopJournal{dir: dir, file: f}, nil
}
func (j *desktopJournal) append(record computer.Record, frameSequence uint64) (computer.Record, []devcouncil.EvidenceArtifact, error) {
	j.records++
	if j.records > 4096 {
		return record, nil, errors.New("run evidence record limit exceeded")
	}
	artifacts := []devcouncil.EvidenceArtifact{}
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
		if err = writeAtomic(filepath.Join(j.dir, prefix+".png"), imageBytes); err != nil {
			return record, nil, err
		}
		artifacts = append(artifacts, devcouncil.EvidenceArtifact{ID: prefix, Path: prefix + ".png", SHA256: digest(imageBytes), SizeBytes: uint64(len(imageBytes))})
		o.Screenshot.Base64 = ""
		treeBytes, err := json.MarshalIndent(o, "", "  ")
		if err != nil {
			return record, nil, err
		}
		if err = writeAtomic(filepath.Join(j.dir, prefix+".json"), treeBytes); err != nil {
			return record, nil, err
		}
		artifacts = append(artifacts, devcouncil.EvidenceArtifact{ID: prefix + "-tree", Path: prefix + ".json", SHA256: digest(treeBytes), SizeBytes: uint64(len(treeBytes))})
		record.Observation = &o
	}
	data, err := json.Marshal(record)
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
	return record, artifacts, j.file.Sync()
}
