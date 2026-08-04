package service

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"nopsai/pkg/proto"
)

const (
	runnerConnectionStatusOnline      = "online"
	runnerConnectionStatusUnreachable = "unreachable"

	runnerMetadataActiveRuns       = "active_runs"
	runnerMetadataConnectionStatus = "connection_status"
	runnerMetadataDisconnectedAt   = "last_disconnected_at"
	runnerMetadataReachable        = "reachable"
	runnerMetadataReachableFalse   = "false"
	runnerMetadataReachableTrue    = "true"
)

type runnerRecord struct {
	id             string
	connectionID   string
	scopes         map[string]struct{}
	capacity       int32
	active         int32
	lastHeartbeat  time.Time
	inflight       map[string]*proto.JobRequest
	metadata       map[string]string
	allowDispatch  bool
	reachable      bool
	disconnectedAt time.Time
}

func connectedRunnerRecord(r *runnerConn) *runnerRecord {
	if r == nil {
		return nil
	}
	return &runnerRecord{
		id:            r.id,
		connectionID:  r.connectionID,
		scopes:        cloneSet(r.scopes),
		capacity:      r.capacity,
		active:        r.active,
		lastHeartbeat: r.lastHeartbeat,
		inflight:      cloneInflightJobs(r.inflight),
		metadata:      cloneMetadata(r.metadata),
		allowDispatch: r.allowDispatch,
		reachable:     true,
	}
}

func (d *dispatcherServer) recordRunnerSnapshotLocked(r *runnerConn, reachable bool, disconnectedAt time.Time) *runnerRecord {
	if r == nil || strings.TrimSpace(r.id) == "" {
		return nil
	}
	if d.registeredRunners == nil {
		d.registeredRunners = make(map[string]*runnerRecord)
	}
	record := d.registeredRunners[r.id]
	if record == nil {
		record = &runnerRecord{id: r.id}
		d.registeredRunners[r.id] = record
	}
	lastDisconnectedAt := record.disconnectedAt

	record.id = r.id
	record.connectionID = r.connectionID
	record.scopes = cloneSet(r.scopes)
	record.capacity = r.capacity
	record.active = r.active
	record.lastHeartbeat = r.lastHeartbeat
	record.inflight = cloneInflightJobs(r.inflight)
	record.metadata = cloneMetadata(r.metadata)
	record.allowDispatch = r.allowDispatch
	record.reachable = reachable
	if reachable {
		record.disconnectedAt = lastDisconnectedAt
	} else if !disconnectedAt.IsZero() {
		record.disconnectedAt = disconnectedAt
	}
	return record
}

func (d *dispatcherServer) markRunnerUnreachableLocked(r *runnerConn, disconnectedAt time.Time) {
	if r == nil {
		return
	}
	if replacement := d.runnerByIDLocked(r.id); replacement != nil {
		d.recordRunnerSnapshotLocked(replacement, true, time.Time{})
		return
	}
	d.recordRunnerSnapshotLocked(r, false, disconnectedAt)
}

func (d *dispatcherServer) registeredRunnerRecordsLocked() []*runnerRecord {
	if len(d.registeredRunners) == 0 {
		return nil
	}
	records := make([]*runnerRecord, 0, len(d.registeredRunners))
	for _, record := range d.registeredRunners {
		if record == nil {
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].id == records[j].id {
			return records[i].connectionID < records[j].connectionID
		}
		return records[i].id < records[j].id
	})
	return records
}

func runnerInfoFromRecord(record *runnerRecord) *proto.RunnerInfo {
	if record == nil {
		return nil
	}
	meta := runnerStatusMetadata(record)
	return &proto.RunnerInfo{
		RunnerId:          record.id,
		Scopes:            keys(record.scopes),
		Capacity:          record.capacity,
		ActiveJobs:        record.active,
		InflightJobs:      int32(len(record.inflight)),
		LastHeartbeatUnix: unixOrZero(record.lastHeartbeat),
		Metadata:          meta,
		AllowDispatch:     record.allowDispatch,
	}
}

func runnerStatusMetadata(record *runnerRecord) map[string]string {
	meta := mergeMetadata(record.metadata, record.connectionID)
	if meta == nil {
		meta = map[string]string{}
	}
	if record.reachable {
		meta[runnerMetadataConnectionStatus] = runnerConnectionStatusOnline
		meta[runnerMetadataReachable] = runnerMetadataReachableTrue
		if !record.disconnectedAt.IsZero() {
			meta[runnerMetadataDisconnectedAt] = record.disconnectedAt.UTC().Format(time.RFC3339)
		} else {
			delete(meta, runnerMetadataDisconnectedAt)
		}
		if activeRuns := activeRunsMetadata(record.inflight); activeRuns != "" {
			meta[runnerMetadataActiveRuns] = activeRuns
		} else {
			delete(meta, runnerMetadataActiveRuns)
		}
		return meta
	}

	meta[runnerMetadataConnectionStatus] = runnerConnectionStatusUnreachable
	meta[runnerMetadataReachable] = runnerMetadataReachableFalse
	if !record.disconnectedAt.IsZero() {
		meta[runnerMetadataDisconnectedAt] = record.disconnectedAt.UTC().Format(time.RFC3339)
	}
	delete(meta, runnerMetadataActiveRuns)
	return meta
}

func activeRunsMetadata(inflight map[string]*proto.JobRequest) string {
	if len(inflight) == 0 {
		return ""
	}
	runSummaries := make([]map[string]string, 0, len(inflight))
	for runID, job := range inflight {
		entry := map[string]string{"run_id": runID}
		if job != nil {
			if name := strings.TrimSpace(job.PipelineName); name != "" {
				entry["pipeline"] = name
			}
			if trig := strings.TrimSpace(job.TriggerEventId); trig != "" {
				entry["trigger_event_id"] = trig
			}
		}
		runSummaries = append(runSummaries, entry)
	}
	sort.Slice(runSummaries, func(i, j int) bool {
		return runSummaries[i]["run_id"] < runSummaries[j]["run_id"]
	})
	data, err := json.Marshal(runSummaries)
	if err != nil {
		return ""
	}
	return string(data)
}

func cloneInflightJobs(inflight map[string]*proto.JobRequest) map[string]*proto.JobRequest {
	if len(inflight) == 0 {
		return map[string]*proto.JobRequest{}
	}
	out := make(map[string]*proto.JobRequest, len(inflight))
	for runID, job := range inflight {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		out[runID] = job
	}
	return out
}

func unixOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}
