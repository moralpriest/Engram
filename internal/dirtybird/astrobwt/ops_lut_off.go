//go:build !lut

package astrobwt

// useLUT is false by default, so the fast path is compiled out entirely and the
// untagged build keeps the branchy switch's existing codegen.
const useLUT = false
