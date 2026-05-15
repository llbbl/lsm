package audit

// Compile-time assertion that FileSink satisfies the Sink interface.
var _ Sink = (*FileSink)(nil)
