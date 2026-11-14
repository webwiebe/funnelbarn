// Package pipeline moves ingest records from the spool to stored events: the
// worker loops that read the spool, the applier that turns a record into rows,
// the forwarders that publish records to the ingest queue, and the health
// monitor that reports a stalled or blind worker.
//
// The monitor is a set of pure state machines that report when something looks
// wrong; the caller decides how to react (typically a slog.Error, which the
// bblog handler forwards to BugBarn as an issue). It covers two failure modes
// that previously produced no signal at all:
//   - the spool consumer stalling (cursor frozen while a backlog accumulates),
//     which once halted all ingestion for ~5 days unnoticed;
//   - geo enrichment being enabled but resolving nothing (e.g. the real client
//     IP never reaching the service), which left country_code empty for every
//     event ever recorded.
package pipeline
