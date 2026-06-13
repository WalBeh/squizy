package workload

// wordBank is a neutral pool of common English words used to synthesize filler
// prompt context of a controlled length. Content is deliberately bland so it
// neither trips safety filters nor biases generation.
var wordBank = []string{
	"system", "process", "value", "model", "result", "context", "method", "signal",
	"network", "memory", "vector", "matrix", "function", "parameter", "gradient", "weight",
	"layer", "token", "sequence", "pattern", "structure", "element", "object", "instance",
	"resource", "channel", "buffer", "stream", "request", "response", "metric", "latency",
	"throughput", "capacity", "schedule", "worker", "thread", "queue", "batch", "window",
	"sample", "average", "median", "summary", "report", "analysis", "measure", "estimate",
	"baseline", "scenario", "workload", "concurrent", "parallel", "sequential", "iteration",
	"document", "section", "chapter", "summary", "detail", "concept", "principle", "approach",
	"design", "interface", "module", "component", "service", "endpoint", "protocol", "format",
	"mountain", "river", "forest", "ocean", "valley", "garden", "morning", "evening", "season",
	"history", "science", "language", "culture", "economy", "industry", "machine", "engine",
}
