package units

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Sentinel errors returned by ParseBytes. Use errors.Is to match them;
// the returned error always quotes the offending input for context.
var (
	// ErrSyntax is returned when the input does not follow the strict
	// "<digits>[.<digits>]<unit>" grammar, e.g. it is empty, carries a
	// sign, uses an exponent, contains spaces or extra symbols, has an
	// unknown or missing unit, or is not a number at all (NaN, Inf).
	ErrSyntax = errors.New("invalid size syntax")

	// ErrOverflow is returned when the value is well-formed but does not
	// fit into a uint64 number of bytes.
	ErrOverflow = errors.New("size overflows uint64")

	// ErrInexact is returned when the value is well-formed and in range
	// but does not resolve to a whole number of bytes (e.g. "0.5B").
	// ParseBytes never rounds, so such inputs are rejected instead of
	// being silently truncated.
	ErrInexact = errors.New("size is not a whole number of bytes")
)

// strictUnits maps the exact, case-sensitive unit suffix to its multiplier
// in bytes. Decimal (SI) units are powers of 1000, binary (IEC) units are
// powers of 1024; the two families are never interchangeable here.
//
// The map is written once at package init and never mutated afterwards,
// so concurrent ParseBytes calls share no mutable state.
var strictUnits = map[string]uint64{
	"B":   1,
	"kB":  KB,
	"KB":  KB,
	"MB":  MB,
	"GB":  GB,
	"TB":  TB,
	"PB":  PB,
	"EB":  EB,
	"KiB": KiB,
	"MiB": MiB,
	"GiB": GiB,
	"TiB": TiB,
	"PiB": PiB,
	"EiB": EiB,
}

// Decimal and binary exa-scale multipliers, kept as untyped constants so
// they also work in 32-bit builds.
const (
	EB  = 1000 * 1000 * 1000 * 1000 * 1000 * 1000
	EiB = 1024 * 1024 * 1024 * 1024 * 1024 * 1024
)

// ParseBytes parses a size specification into an exact number of bytes.
//
// Unlike FromHumanSize and RAMInBytes, which go through float64 and may
// round large or fractional values, ParseBytes uses exact integer
// arithmetic only and never loses precision. The grammar is deliberately
// strict:
//
//		<digits>[.<digits>]<unit>
//
//	  - The number must be unsigned decimal digits with at most one dot;
//	    signs, exponents, underscores, spaces and non-numbers (NaN, Inf)
//	    are rejected.
//	  - The unit is mandatory and case-sensitive: "B", the decimal SI
//	    units "kB"/"KB", "MB", "GB", "TB", "PB", "EB" (powers of 1000),
//	    or the binary IEC units "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"
//	    (powers of 1024).
//
// The result must be a whole number of bytes that fits in a uint64;
// otherwise an error wrapping ErrInexact or ErrOverflow is returned.
// Syntax violations return an error wrapping ErrSyntax. In all cases the
// error message quotes the original input.
//
// ParseBytes has no shared mutable state and is safe for concurrent use.
func ParseBytes(s string) (uint64, error) {
	num, unit := splitStrictSize(s)
	if num == "" {
		return 0, fmt.Errorf("units: cannot parse %q: %w", s, ErrSyntax)
	}
	mul, ok := strictUnits[unit]
	if !ok {
		return 0, fmt.Errorf("units: cannot parse %q: unknown unit %q: %w", s, unit, ErrSyntax)
	}

	intPart, fracPart, ok := splitStrictNumber(num)
	if !ok {
		return 0, fmt.Errorf("units: cannot parse %q: malformed number %q: %w", s, num, ErrSyntax)
	}

	// Exact arithmetic: value = (intPart.fracPart) * mul
	//                   = digits * mul / 10^len(fracPart).
	mantissa, _ := new(big.Int).SetString(intPart+fracPart, 10)
	mantissa.Mul(mantissa, new(big.Int).SetUint64(mul))
	if len(fracPart) > 0 {
		den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(fracPart))), nil)
		q, r := new(big.Int).QuoRem(mantissa, den, new(big.Int))
		if r.Sign() != 0 {
			return 0, fmt.Errorf("units: cannot parse %q: %w", s, ErrInexact)
		}
		mantissa = q
	}
	if !mantissa.IsUint64() {
		return 0, fmt.Errorf("units: cannot parse %q: %w", s, ErrOverflow)
	}
	return mantissa.Uint64(), nil
}

// splitStrictSize splits s into its leading numeric run and the remaining
// unit suffix. It reports an empty number when s has no leading digit or
// dot, which the caller treats as a syntax error.
func splitStrictSize(s string) (num, unit string) {
	i := strings.IndexFunc(s, func(r rune) bool {
		return r != '.' && (r < '0' || r > '9')
	})
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}

// splitStrictNumber splits num into its integer and fractional digit runs.
// It requires at least one digit on each side of an optional single dot,
// and rejects anything else (multiple dots, lone dots, empty sides).
func splitStrictNumber(num string) (intPart, fracPart string, ok bool) {
	intPart, fracPart, hasDot := strings.Cut(num, ".")
	if intPart == "" || !isDigits(intPart) {
		return "", "", false
	}
	if hasDot && (fracPart == "" || !isDigits(fracPart)) {
		return "", "", false
	}
	return intPart, fracPart, true
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// FormatBytes returns the canonical, exact string form of a byte count,
// such that ParseBytes(FormatBytes(v)) == v for every uint64 v.
//
// The formatting rule is fixed: among all units that divide v exactly,
// the largest one (in bytes) wins, so the mantissa is always a whole
// number and no rounding ever takes place. Values not divisible by any
// unit are emitted in bytes. Zero formats as "0B".
func FormatBytes(v uint64) string {
	// Ordered largest-first; the first exact divisor wins. Binary and
	// decimal units of different scale never coincide, so the choice is
	// unambiguous.
	for _, u := range []struct {
		suffix string
		mul    uint64
	}{
		{"EiB", EiB}, {"EB", EB},
		{"PiB", PiB}, {"PB", PB},
		{"TiB", TiB}, {"TB", TB},
		{"GiB", GiB}, {"GB", GB},
		{"MiB", MiB}, {"MB", MB},
		{"KiB", KiB}, {"kB", KB},
	} {
		if v >= u.mul && v%u.mul == 0 {
			return fmt.Sprintf("%d%s", v/u.mul, u.suffix)
		}
	}
	return fmt.Sprintf("%dB", v)
}
