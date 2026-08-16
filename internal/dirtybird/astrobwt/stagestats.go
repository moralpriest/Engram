package astrobwt

// Stage indices for the optional per-stage cycle accounting (build tag
// "stagestats"). Release builds compile the stageMark/stageLap hooks in
// pow.go to nothing.
const (
	stagePrologue = iota // SHA-256 -> Salsa20 -> RC4 -> fnv1a setup
	stageWolf            // the 256-op branchy loop incl. marker flush
	stageSA              // suffix-array build (v114 or SAIS)
	stageSHA             // final SHA-256 over the SA bytes
	stageCount
)
