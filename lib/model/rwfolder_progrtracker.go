// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"time"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

const (
	progressEventInterval = 2500 * time.Millisecond
)

// The progressTracker generates ItemStarted, ItemFinished and
// DownloadProgress events based on the progress information received from the
// ChangeSet.
type progressTracker struct {
	folder   string                    // the folder we're tracking
	files    map[string]pullerProgress // the files we're tracking
	lastEmit time.Time                 // when we last emitted a DownloadProgress event
	mut      sync.Mutex                // protects the above
}

// A momentary state representing the progress of the puller
type pullerProgress struct {
	Total      int   `json:"total"`
	Reused     int   `json:"reused"`
	Copied     int   `json:"copied"`
	Pulled     int   `json:"pulled"`
	Pulling    int   `json:"pulling"`
	BytesDone  int64 `json:"bytesDone"`
	BytesTotal int64 `json:"bytesTotal"`
}

func newProgressTracker(folder string) *progressTracker {
	return &progressTracker{
		folder: folder,
		files:  make(map[string]pullerProgress),
		mut:    sync.NewMutex(),
	}
}

func (p *progressTracker) Started(file protocol.FileInfo) {
	events.Default.Log(events.ItemStarted, p.eventData(file))
}

func (p *progressTracker) Progress(file protocol.FileInfo, copied, requested, downloaded int) {
	p.mut.Lock()
	cur := p.files[file.Name]
	cur.Copied += copied
	cur.Pulling += requested
	cur.Pulled += downloaded
	p.files[file.Name] = cur

	if time.Since(p.lastEmit) > progressEventInterval {
		p.emitDownloadProgress()
	}
	p.mut.Unlock()
}

func (p *progressTracker) Completed(file protocol.FileInfo, err error) {
	data := p.eventData(file)
	data["error"] = events.Error(err)
	events.Default.Log(events.ItemFinished, data)

	p.mut.Lock()
	delete(p.files, file.Name)

	if time.Since(p.lastEmit) > progressEventInterval {
		p.emitDownloadProgress()
	}
	p.mut.Unlock()
}

func (p *progressTracker) eventData(file protocol.FileInfo) map[string]interface{} {
	ftype := "file"
	if file.IsDirectory() {
		ftype = "dir"
	}

	action := "update"
	if file.IsDeleted() {
		action = "delete"
	}

	return map[string]interface{}{
		"folder": p.folder,
		"item":   file.Name,
		"type":   ftype,
		"action": action,
	}
}

func (p *progressTracker) emitDownloadProgress() {
	// Must be called with p.mut held

	// Copy the map, as it would otherwise suffer a race condition when we
	// modify it while it's in the event queue.
	mapCopy := make(map[string]pullerProgress, len(p.files))
	for file, progress := range p.files {
		mapCopy[file] = progress
	}

	events.Default.Log(events.DownloadProgress, map[string]map[string]pullerProgress{
		p.folder: mapCopy,
	})

	p.lastEmit = time.Now()
}