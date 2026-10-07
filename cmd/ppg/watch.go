package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"
)

// corpusFingerprint summarizes every file under the corpus directories
// (path, size, modification time). Two equal fingerprints mean nothing on
// disk changed since the last load; reading content is not needed because a
// writer always moves the mtime or the size. A missing directory contributes
// nothing, so it appearing later is a change.
func corpusFingerprint(dirs ...string) string {
	h := sha256.New()
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			fmt.Fprintf(h, "%s\x00%d\x00%d\n", p, info.Size(), info.ModTime().UnixNano())
			return nil
		})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// watchCorpus polls the corpus directories every interval and sends on
// changed once their fingerprint has moved and then held still for one tick
// — the writer (a generator rewriting a .rego, an editor saving) is done, so
// the reload does not read a half-written file. Polling rather than
// inotify/kqueue: no dependency, identical on every OS, and a corpus changes
// a few times a day — a few seconds of delay is invisible to the human who
// just validated a plan.
func watchCorpus(interval time.Duration, changed chan<- struct{}, dirs ...string) {
	loaded := corpusFingerprint(dirs...)
	previous := loaded
	for range time.Tick(interval) {
		fp := corpusFingerprint(dirs...)
		if fp == previous && fp != loaded {
			loaded = fp
			select {
			case changed <- struct{}{}:
			default: // a reload is already pending; it will read the latest files
			}
		}
		previous = fp
	}
}
