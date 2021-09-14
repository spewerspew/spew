/*
 * Copyright (c) 2013-2016 Dave Collins <dave@davec.name>
 * Copyright (c) 2021 Anner van Hardenbroek
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package spew

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"sync"
)

// Some constants in the form of bytes to avoid string overhead.  This mirrors
// the technique used in the fmt package.
var (
	panicBytes            = []byte("(PANIC=")
	plusBytes             = []byte("+")
	iBytes                = []byte("i")
	trueBytes             = []byte("true")
	falseBytes            = []byte("false")
	interfaceBytes        = []byte("(interface {})")
	commaNewlineBytes     = []byte(",\n")
	newlineBytes          = []byte("\n")
	openBraceBytes        = []byte("{")
	openBraceNewlineBytes = []byte("{\n")
	closeBraceBytes       = []byte("}")
	asteriskBytes         = []byte("*")
	colonBytes            = []byte(":")
	colonSpaceBytes       = []byte(": ")
	openParenBytes        = []byte("(")
	closeParenBytes       = []byte(")")
	spaceBytes            = []byte(" ")
	pointerChainBytes     = []byte("->")
	nilAngleBytes         = []byte("<nil>")
	maxNewlineBytes       = []byte("<max depth reached>\n")
	maxShortBytes         = []byte("<max>")
	circularBytes         = []byte("<already shown>")
	circularShortBytes    = []byte("<shown>")
	invalidAngleBytes     = []byte("<invalid>")
	openBracketBytes      = []byte("[")
	closeBracketBytes     = []byte("]")
	percentBytes          = []byte("%")
	precisionBytes        = []byte(".")
	openAngleBytes        = []byte("<")
	closeAngleBytes       = []byte(">")
	openMapBytes          = []byte("map[")
	closeMapBytes         = []byte("]")
	lenEqualsBytes        = []byte("len=")
	capEqualsBytes        = []byte("cap=")
)

// hexDigits is used to map a decimal value to a hex digit.
const hexDigits = "0123456789abcdef"

// catchPanic handles any panics that might occur during the handleMethods
// calls.
func catchPanic(w io.Writer, v reflect.Value) {
	if err := recover(); err != nil {
		w.Write(panicBytes)
		fmt.Fprintf(w, "%v", err)
		w.Write(closeParenBytes)
	}
}

// handleMethods attempts to call the Error and String methods on the underlying
// type the passed reflect.Value represents and outputes the result to Writer w.
//
// It handles panics in any called methods by catching and displaying the error
// as the formatted value.
func handleMethods(cs *ConfigState, w io.Writer, v reflect.Value) (handled bool) {
	// We need an interface to check if the type implements the error or
	// Stringer interface.  However, the reflect package won't give us an
	// interface on certain things like unexported struct fields in order
	// to enforce visibility rules.  We use unsafe, when it's available,
	// to bypass these restrictions since this package does not mutate the
	// values.
	if !v.CanInterface() {
		if UnsafeDisabled {
			return false
		}

		v = unsafeReflectValue(v)
	}

	// Choose whether or not to do error and Stringer interface lookups against
	// the base type or a pointer to the base type depending on settings.
	// Technically calling one of these methods with a pointer receiver can
	// mutate the value, however, types which choose to satisify an error or
	// Stringer interface with a pointer receiver should not be mutating their
	// state inside these interface methods.
	if !cs.DisablePointerMethods && !UnsafeDisabled && !v.CanAddr() {
		v = unsafeReflectValue(v)
	}
	if v.CanAddr() {
		v = v.Addr()
	}

	// Is it an error or Stringer?
	switch iface := v.Interface().(type) {
	case error:
		defer catchPanic(w, v)
		if cs.ContinueOnMethod {
			w.Write(openParenBytes)
			io.WriteString(w, iface.Error())
			w.Write(closeParenBytes)
			w.Write(spaceBytes)
			return false
		}
		io.WriteString(w, iface.Error())
		return true

	case fmt.Stringer:
		defer catchPanic(w, v)
		if cs.ContinueOnMethod {
			w.Write(openParenBytes)
			io.WriteString(w, iface.String())
			w.Write(closeParenBytes)
			w.Write(spaceBytes)
			return false
		}
		io.WriteString(w, iface.String())
		return true
	}
	return false
}

// printBool outputs a boolean value as true or false to Writer w.
func printBool(w io.Writer, val bool) {
	if val {
		w.Write(trueBytes)
	} else {
		w.Write(falseBytes)
	}
}

// printInt outputs a signed integer value to Writer w.
func printInt(w io.Writer, val int64, base int) {
	pv, buf := strconvBufPoolGet()
	defer strconvBufPool.Put(pv)
	w.Write(strconv.AppendInt(buf, val, base))
}

// printUint outputs an unsigned integer value to Writer w.
func printUint(w io.Writer, val uint64, base int) {
	pv, buf := strconvBufPoolGet()
	defer strconvBufPool.Put(pv)
	w.Write(strconv.AppendUint(buf, val, base))
}

// printFloat outputs a floating point value using the specified precision,
// which is expected to be 32 or 64bit, to Writer w.
func printFloat(w io.Writer, val float64, precision int) {
	pv, buf := strconvBufPoolGet()
	defer strconvBufPool.Put(pv)
	w.Write(strconv.AppendFloat(buf, val, 'g', -1, precision))
}

// printComplex outputs a complex value using the specified float precision
// for the real and imaginary parts to Writer w.
func printComplex(w io.Writer, c complex128, floatPrecision int) {
	pv, buf := strconvBufPoolGet()
	defer strconvBufPool.Put(pv)
	r := real(c)
	w.Write(openParenBytes)
	w.Write(strconv.AppendFloat(buf, r, 'g', -1, floatPrecision))
	i := imag(c)
	if i >= 0 {
		w.Write(plusBytes)
	}
	buf = buf[:0]
	w.Write(strconv.AppendFloat(buf, i, 'g', -1, floatPrecision))
	w.Write(iBytes)
	w.Write(closeParenBytes)
}

var strconvBufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 0, 64)
	},
}

func strconvBufPoolGet() (interface{}, []byte) {
	pv := strconvBufPool.Get()
	buf := pv.([]byte)
	buf = buf[:0]
	return pv, buf
}

// printHexPtr outputs a uintptr formatted as hexadecimal with a leading '0x'
// prefix to Writer w.
func printHexPtr(w io.Writer, p uintptr) {
	// Null pointer.
	num := uint64(p)
	if num == 0 {
		w.Write(nilAngleBytes)
		return
	}

	// Max uint64 is 16 bytes in hex + 2 bytes for '0x' prefix
	buf := printHexPtrBufGet() // *[18]byte
	defer printHexPtrBufPut(buf)

	// It's simpler to construct the hex string right to left.
	base := uint64(16)
	i := len(buf) - 1
	for num >= base {
		buf[i] = hexDigits[num%base]
		num /= base
		i--
	}
	buf[i] = hexDigits[num]

	// Add '0x' prefix.
	i--
	buf[i] = 'x'
	i--
	buf[i] = '0'

	// Strip unused leading bytes and write.
	w.Write(buf[i:])
}

var printHexPtrBufPool = sync.Pool{
	New: func() interface{} {
		return new([18]byte)
	},
}

func printHexPtrBufPut(buf *[18]byte) { printHexPtrBufPool.Put(buf) }
func printHexPtrBufGet() *[18]byte {
	return printHexPtrBufPool.Get().(*[18]byte)
}

// cycleInfo stores circular pointer information.
type cycleInfo struct {
	pointers     map[uintptr]int
	pointerChain []uintptr
	nilFound     bool
	cycleFound   bool
	indirects    int
}

func (ci *cycleInfo) Reset() {
	if ci == nil {
		return
	}
	for k := range ci.pointers {
		delete(ci.pointers, k)
	}
	*ci = cycleInfo{
		pointers:     ci.pointers,
		pointerChain: ci.pointerChain[:0],
	}
}

var cycleInfoPool = sync.Pool{New: func() interface{} {
	return &cycleInfo{pointers: make(map[uintptr]int)}
}}

func cycleInfoGet() *cycleInfo {
	ci := cycleInfoPool.Get().(*cycleInfo)
	ci.Reset()
	return ci
}

func cycleInfoPut(ci *cycleInfo) {
	if ci != nil {
		cycleInfoPool.Put(ci)
	}
}

// derefPtr dereferences a pointer and unpacks interfaces down
// the chain while detecting circular references.
func derefPtr(v reflect.Value, depth int, ci *cycleInfo) reflect.Value {
	ci.pointerChain = ci.pointerChain[:0]

	// Remove pointers at or below the current depth from map used to detect
	// circular refs.
	for k, d := range ci.pointers {
		if d >= depth {
			delete(ci.pointers, k)
		}
	}

	// Figure out how many levels of indirection there are by dereferencing
	// pointers and unpacking interfaces down the chain while detecting circular
	// references.
	ci.nilFound = false
	ci.cycleFound = false
	ci.indirects = 0
	ve := v
	for ve.Kind() == reflect.Ptr {
		if ve.IsNil() {
			ci.nilFound = true
			break
		}
		ci.indirects++
		addr := ve.Pointer()
		ci.pointerChain = append(ci.pointerChain, addr)
		if pd, ok := ci.pointers[addr]; ok && pd < depth {
			ci.cycleFound = true
			ci.indirects--
			break
		}
		ci.pointers[addr] = depth

		ve = ve.Elem()
		if ve.Kind() == reflect.Interface {
			if ve.IsNil() {
				ci.nilFound = true
				break
			}
			ve = ve.Elem()
		}
	}
	return ve
}

// valuesSorter implements sort.Interface to allow a slice of reflect.Value
// elements to be sorted.
type valuesSorter struct {
	values  []reflect.Value
	strings []string // either nil or same len as values
	cs      *ConfigState
}

// newValuesSorter initializes a valuesSorter instance, which holds a set of
// surrogate keys on which the data should be sorted.  It uses flags in
// ConfigState to decide if and how to populate those surrogate keys.
func newValuesSorter(values []reflect.Value, cs *ConfigState) sort.Interface {
	vs := &valuesSorter{values: values, cs: cs}
	if canSortSimply(vs.values[0].Kind()) {
		return vs
	}
	if !cs.DisableMethods {
		vs.strings = make([]string, len(values))
		for i := range vs.values {
			var b bytes.Buffer
			if !handleMethods(cs, &b, vs.values[i]) {
				vs.strings = nil
				break
			}
			vs.strings[i] = b.String()
		}
	}
	if vs.strings == nil && cs.SpewKeys {
		vs.strings = make([]string, len(values))
		for i := range vs.values {
			vs.strings[i] = Sprintf("%#v", vs.values[i].Interface())
		}
	}
	return vs
}

// canSortSimply tests whether a reflect.Kind is a primitive that can be sorted
// directly, or whether it should be considered for sorting by surrogate keys
// (if the ConfigState allows it).
func canSortSimply(kind reflect.Kind) bool {
	// This switch parallels valueSortLess, except for the default case.
	switch kind {
	case reflect.Bool:
		return true
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		return true
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	case reflect.String:
		return true
	case reflect.Uintptr:
		return true
	case reflect.Array:
		return true
	}
	return false
}

// Len returns the number of values in the slice.  It is part of the
// sort.Interface implementation.
func (s *valuesSorter) Len() int {
	return len(s.values)
}

// Swap swaps the values at the passed indices.  It is part of the
// sort.Interface implementation.
func (s *valuesSorter) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	if s.strings != nil {
		s.strings[i], s.strings[j] = s.strings[j], s.strings[i]
	}
}

// valueSortLess returns whether the first value should sort before the second
// value.  It is used by valueSorter.Less as part of the sort.Interface
// implementation.
func valueSortLess(a, b reflect.Value) bool {
	switch a.Kind() {
	case reflect.Bool:
		return !a.Bool() && b.Bool()
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		return a.Int() < b.Int()
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		return a.Uint() < b.Uint()
	case reflect.Float32, reflect.Float64:
		return a.Float() < b.Float()
	case reflect.String:
		return a.String() < b.String()
	case reflect.Uintptr:
		return a.Uint() < b.Uint()
	case reflect.Array:
		// Compare the contents of both arrays.
		l := a.Len()
		for i := 0; i < l; i++ {
			av := a.Index(i)
			bv := b.Index(i)
			if av.Interface() == bv.Interface() {
				continue
			}
			return valueSortLess(av, bv)
		}
	}
	return a.String() < b.String()
}

// Less returns whether the value at index i should sort before the
// value at index j.  It is part of the sort.Interface implementation.
func (s *valuesSorter) Less(i, j int) bool {
	if s.strings == nil {
		return valueSortLess(s.values[i], s.values[j])
	}
	return s.strings[i] < s.strings[j]
}

// sortValues is a sort function that handles both native types and any type that
// can be converted to error or Stringer.  Other inputs are sorted according to
// their Value.String() value to ensure display stability.
func sortValues(values []reflect.Value, cs *ConfigState) {
	if len(values) == 0 {
		return
	}
	sort.Sort(newValuesSorter(values, cs))
}

var bytesBufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func bytesBufferPut(b *bytes.Buffer) { bytesBufferPool.Put(b) }
func bytesBufferGet() *bytes.Buffer {
	b := bytesBufferPool.Get().(*bytes.Buffer)
	b.Reset()
	return b
}
