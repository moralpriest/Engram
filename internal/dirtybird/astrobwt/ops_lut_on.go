//go:build lut

package astrobwt

// useLUT routes the 149 single-byte ops through opLUT instead of the switch.
const useLUT = true
