package units

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
)

func TestParseBytes(t *testing.T) {
	for _, tc := range []struct {
		in       string
		expected uint64
	}{
		// Bytes.
		{"0B", 0},
		{"1B", 1},
		{"32B", 32},
		{"00032B", 32},

		// Decimal (SI) units, base 1000.
		{"1kB", KB},
		{"1KB", KB},
		{"32kB", 32 * KB},
		{"32MB", 32 * MB},
		{"32GB", 32 * GB},
		{"32TB", 32 * TB},
		{"32PB", 32 * PB},
		{"1EB", EB},

		// Binary (IEC) units, base 1024.
		{"1KiB", KiB},
		{"32KiB", 32 * KiB},
		{"32MiB", 32 * MiB},
		{"32GiB", 32 * GiB},
		{"32TiB", 32 * TiB},
		{"32PiB", 32 * PiB},
		{"1EiB", EiB},

		// Decimal fractions, computed exactly.
		{"1.5KiB", 1536},
		{"0.5MiB", 512 * KiB},
		{"32.5kB", 32500},
		{"0.3kB", 300},
		{"2.5EB", 2500 * PB},
		{"1.5EiB", EiB + EiB/2},
		{"0.000001MB", 1}, // exactly one byte
		{"18446744073709551615B", math.MaxUint64},

		// Values that lose precision when parsed via float64 must be
		// exact here: 9007199254740993 is not representable as float64.
		{"9007199254740993B", 9007199254740993},
		{"9007199.254740993GB", 9007199254740993},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseBytes(tc.in)
			if err != nil {
				t.Fatalf("ParseBytes(%q) returned error: %v", tc.in, err)
			}
			if got != tc.expected {
				t.Errorf("ParseBytes(%q) = %d, expected %d", tc.in, got, tc.expected)
			}
		})
	}
}

func TestParseBytesSyntaxErrors(t *testing.T) {
	for _, in := range []string{
		"",        // empty
		"B",       // missing number
		"kB",      // missing number
		" ",       // whitespace only
		" 1B",     // leading space
		"1B ",     // trailing space
		"1 B",     // inner space
		"1",       // missing unit (ambiguous)
		"32",      // missing unit (ambiguous)
		"-1B",     // negative
		"-0B",     // negative zero
		"+1B",     // explicit sign
		"NaN",     // not a number
		"Inf",     // infinity
		"-Inf",    // negative infinity
		"1e3B",    // exponent notation
		"1_000B",  // underscores
		".5B",     // missing integer part
		"1.B",     // missing fractional part
		".B",      // lone dot
		"1.2.3B",  // multiple dots
		"1b",      // lowercase b is not a unit
		"1k",      // bare k is ambiguous
		"1K",      // bare K is ambiguous
		"1M",      // bare M is ambiguous
		"1kb",     // lowercase suffix
		"1mB",     // lowercase prefix
		"1KiBB",   // extra symbol after unit
		"1BB",     // extra symbol after unit
		"1B B",    // extra unit
		"1.5KiBB", // extra symbol after binary unit
		"0x10B",   // hex
		"１B",      // non-ASCII digit
	} {
		t.Run(in, func(t *testing.T) {
			got, err := ParseBytes(in)
			if err == nil {
				t.Fatalf("ParseBytes(%q) = %d, expected error", in, got)
			}
			if !errors.Is(err, ErrSyntax) {
				t.Errorf("ParseBytes(%q) error %v does not wrap ErrSyntax", in, err)
			}
			if !strings.Contains(err.Error(), in) {
				t.Errorf("ParseBytes(%q) error %q does not quote the input", in, err)
			}
		})
	}
}

func TestParseBytesOverflow(t *testing.T) {
	for _, in := range []string{
		"18446744073709551616B", // MaxUint64 + 1
		"16EiB",                 // 2^64
		"18.446744073709551616EB",
		"100000EB",
		"99999999999999999999B",
		"1000000000EiB",
	} {
		t.Run(in, func(t *testing.T) {
			got, err := ParseBytes(in)
			if err == nil {
				t.Fatalf("ParseBytes(%q) = %d, expected overflow error", in, got)
			}
			if !errors.Is(err, ErrOverflow) {
				t.Errorf("ParseBytes(%q) error %v does not wrap ErrOverflow", in, err)
			}
			if !strings.Contains(err.Error(), in) {
				t.Errorf("ParseBytes(%q) error %q does not quote the input", in, err)
			}
		})
	}
}

func TestParseBytesInexact(t *testing.T) {
	for _, in := range []string{
		"0.5B",        // half a byte
		"0.1B",        // tenth of a byte
		"1.0001kB",    // 1000.1 bytes
		"0.0000001MB", // 0.1 bytes
	} {
		t.Run(in, func(t *testing.T) {
			got, err := ParseBytes(in)
			if err == nil {
				t.Fatalf("ParseBytes(%q) = %d, expected inexact error", in, got)
			}
			if !errors.Is(err, ErrInexact) {
				t.Errorf("ParseBytes(%q) error %v does not wrap ErrInexact", in, err)
			}
			if !strings.Contains(err.Error(), in) {
				t.Errorf("ParseBytes(%q) error %q does not quote the input", in, err)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	for _, tc := range []struct {
		in       uint64
		expected string
	}{
		{0, "0B"},
		{1, "1B"},
		{999, "999B"},
		{1000, "1kB"},
		{1024, "1KiB"},
		{1536, "1536B"}, // 1.5 KiB is not a whole unit, so bytes win
		{2000, "2kB"},
		{2048, "2KiB"},
		{MB, "1MB"},
		{MiB, "1MiB"},
		{GB, "1GB"},
		{GiB, "1GiB"},
		{TB, "1TB"},
		{TiB, "1TiB"},
		{PB, "1PB"},
		{PiB, "1PiB"},
		{EB, "1EB"},
		{EiB, "1EiB"},
		{1000 * 1024, "1000KiB"}, // largest exact divisor wins
		{math.MaxUint64, "18446744073709551615B"},
	} {
		t.Run(fmt.Sprint(tc.in), func(t *testing.T) {
			if got := FormatBytes(tc.in); got != tc.expected {
				t.Errorf("FormatBytes(%d) = %q, expected %q", tc.in, got, tc.expected)
			}
		})
	}
}

// TestFormatBytesRoundTrip verifies that formatting followed by parsing
// is the identity over the whole representable range.
func TestFormatBytesRoundTrip(t *testing.T) {
	values := []uint64{0, 1, 2, 999, 1000, 1023, 1024, 1025, 1536, 2000, 2048,
		KB, KiB, MB, MiB, GB, GiB, TB, TiB, PB, PiB, EB, EiB,
		32 * KB, 32 * KiB, 32500, math.MaxUint64, math.MaxUint64 - 1,
		math.MaxUint64 / 2, 1<<60 + 1, 1_000_000_000_000_000_001}
	// Deterministic pseudo-random sweep across the full uint64 range.
	for x := uint64(0x9e3779b97f4a7c15); x != 0; x = x*6364136223846793005 + 1442695040888963407 {
		values = append(values, x)
		if len(values) > 2000 {
			break
		}
	}
	for _, v := range values {
		s := FormatBytes(v)
		got, err := ParseBytes(s)
		if err != nil {
			t.Fatalf("ParseBytes(FormatBytes(%d)=%q) returned error: %v", v, s, err)
		}
		if got != v {
			t.Errorf("round trip failed: ParseBytes(FormatBytes(%d)=%q) = %d", v, s, got)
		}
	}
}

// TestParseBytesConcurrent exercises the new entry points from many
// goroutines; run with -race to prove there is no shared mutable state.
func TestParseBytesConcurrent(t *testing.T) {
	inputs := []string{"1B", "1.5KiB", "32.5kB", "2.5EB", "1EiB", "9007199254740993B"}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := inputs[i%len(inputs)]
			want, err := ParseBytes(in)
			if err != nil {
				t.Errorf("ParseBytes(%q) returned error: %v", in, err)
				return
			}
			for j := 0; j < 1000; j++ {
				got, err := ParseBytes(in)
				if err != nil || got != want {
					t.Errorf("ParseBytes(%q) = %d, %v; expected %d", in, got, err, want)
					return
				}
				if rt, err := ParseBytes(FormatBytes(want)); err != nil || rt != want {
					t.Errorf("round trip of %d failed: %d, %v", want, rt, err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

func BenchmarkParseBytes(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, s := range []string{"1B", "1.5KiB", "32.5kB", "32MB", "2.5EB", "1EiB"} {
			if _, err := ParseBytes(s); err != nil {
				b.Fatal(err)
			}
		}
	}
}
