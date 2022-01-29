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
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var (
	// uint8Type is a reflect.Type representing a uint8.  It is used to
	// convert cgo types to uint8 slices for hexdumping.
	uint8Type = reflect.TypeOf(uint8(0))

	// fmtStringerType is a reflect.Type representing fmt.Stringer.
	// It is used to pretty print types instead of hexdump them.
	fmtStringerType = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()

	// cCharRE is a regular expression that matches a cgo char.
	// It is used to detect character arrays to hexdump them.
	cCharRE = regexp.MustCompile(`^.*\._Ctype_char$`)

	// cUnsignedCharRE is a regular expression that matches a cgo unsigned
	// char.  It is used to detect unsigned character arrays to hexdump
	// them.
	cUnsignedCharRE = regexp.MustCompile(`^.*\._Ctype_unsignedchar$`)

	// cUint8tCharRE is a regular expression that matches a cgo uint8_t.
	// It is used to detect uint8_t arrays to hexdump them.
	cUint8tCharRE = regexp.MustCompile(`^.*\._Ctype_uint8_t$`)
)

// indentCacheKey is used as key in the indent cache.
type indentCacheKey struct {
	indent string
	depth  int
}

// indentCache is a cache that caches indents of different lengths.
var indentCache sync.Map // map[indentCacheKey]string

// dumpState contains information about the state of a dump operation.
type dumpState struct {
	w                io.Writer
	depth            int
	ignoreNextType   bool
	ignoreNextIndent bool
	ci               *cycleInfo
	cs               *ConfigState
}

// indent performs indentation according to the depth level and cs.Indent
// option.
func (d *dumpState) indent() {
	if d.ignoreNextIndent {
		d.ignoreNextIndent = false
		return
	}
	for i := 0; i < d.depth; i++ {
		io.WriteString(d.w, d.cs.Indent)
	}
}

// unpackValue returns values inside of non-nil interfaces when possible.
// This is useful for data types like structs, arrays, slices, and maps which
// can contain varying types packed inside an interface.
func (d *dumpState) unpackValue(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Interface && !v.IsNil() {
		v = v.Elem()
	}
	return v
}

// dumpPtr handles formatting of pointers by indirecting them as necessary.
func (d *dumpState) dumpPtr(v reflect.Value) {
	// Figure out how many levels of indirection there are by dereferencing
	// pointers and unpacking interfaces down the chain while detecting circular
	// references.
	ve := derefPtr(v, d.depth, d.ci)

	// Display type information.
	d.w.Write(openParenBytes)
	for i := 0; i < d.ci.indirects; i++ {
		d.w.Write(asteriskBytes)
	}
	io.WriteString(d.w, ve.Type().String())
	d.w.Write(closeParenBytes)

	// Display pointer information.
	if !d.cs.DisablePointerAddresses && len(d.ci.pointerChain) > 0 {
		d.w.Write(openParenBytes)
		for i, addr := range d.ci.pointerChain {
			if i > 0 {
				d.w.Write(pointerChainBytes)
			}
			printHexPtr(d.w, addr)
		}
		d.w.Write(closeParenBytes)
	}

	// Display dereferenced value.
	d.w.Write(openParenBytes)
	switch {
	case d.ci.nilFound:
		d.w.Write(nilAngleBytes)

	case d.ci.cycleFound:
		d.w.Write(circularBytes)

	default:
		d.ignoreNextType = true
		d.dump(ve)
	}
	d.w.Write(closeParenBytes)
}

// dumpSlice handles formatting of arrays and slices.  Byte (uint8 under
// reflection) arrays and slices are dumped in hexdump -C fashion.
func (d *dumpState) dumpSlice(v reflect.Value) {
	// Determine whether this type should be hex dumped or not.  Also,
	// for types which should be hexdumped, try to use the underlying data
	// first, then fall back to trying to convert them to a uint8 slice.
	var buf []uint8
	doConvert := false
	doHexDump := false
	numEntries := v.Len()
	if numEntries > 0 {
		vt := v.Index(0).Type()
		vts := vt.String()
		switch {
		// C types that need to be converted.
		case cCharRE.MatchString(vts):
			fallthrough
		case cUnsignedCharRE.MatchString(vts):
			fallthrough
		case cUint8tCharRE.MatchString(vts):
			doConvert = true

		// Try to use existing uint8 slices and fall back to converting
		// and copying if that fails.
		case vt.Kind() == reflect.Uint8:
			if vt.Implements(fmtStringerType) {
				doConvert = d.cs.DisableMethods
			} else {
				// We need an addressable interface to convert the type
				// to a byte slice.  However, the reflect package won't
				// give us an interface on certain things like
				// unexported struct fields in order to enforce
				// visibility rules.  We use unsafe, when available, to
				// bypass these restrictions since this package does not
				// mutate the values.
				vs := v
				if !vs.CanInterface() || !vs.CanAddr() {
					vs = unsafeReflectValue(vs)
				}
				if !UnsafeDisabled {
					vs = vs.Slice(0, numEntries)

					// Use the existing uint8 slice if it can be
					// type asserted.
					iface := vs.Interface()
					if slice, ok := iface.([]uint8); ok {
						buf = slice
						doHexDump = true
						break
					}
				}

				// The underlying data needs to be converted if it can't
				// be type asserted to a uint8 slice.
				doConvert = true
			}
		}

		// Copy and convert the underlying type if needed.
		if doConvert && vt.ConvertibleTo(uint8Type) {
			// Convert and copy each element into a uint8 byte
			// slice.
			buf = make([]uint8, numEntries)
			for i := 0; i < numEntries; i++ {
				vv := v.Index(i)
				buf[i] = uint8(vv.Convert(uint8Type).Uint())
			}
			doHexDump = true
		}
	}

	// Hexdump the entire slice as needed.
	if doHexDump {
		var indent string
		key := indentCacheKey{d.cs.Indent, d.depth}
		if cv, ok := indentCache.Load(key); ok {
			indent = cv.(string)
		} else {
			indent = strings.Repeat(d.cs.Indent, d.depth)
			indentCache.Store(key, indent)
		}
		hexDump(d.w, buf, indent)
		return
	}

	// Recursively call dump for each item.
	for i := 0; i < numEntries; i++ {
		d.dump(d.unpackValue(v.Index(i)))
		if i < (numEntries - 1) {
			d.w.Write(commaNewlineBytes)
		} else {
			d.w.Write(newlineBytes)
		}
	}
}

// dump is the main workhorse for dumping a value.  It uses the passed reflect
// value to figure out what kind of object we are dealing with and formats it
// appropriately.  It is a recursive function, however circular data structures
// are detected and handled properly.
func (d *dumpState) dump(v reflect.Value) {
	// Handle invalid reflect values immediately.
	kind := v.Kind()
	if kind == reflect.Invalid {
		d.w.Write(invalidAngleBytes)
		return
	}

	// Handle pointers specially.
	if kind == reflect.Ptr {
		d.indent()
		d.dumpPtr(v)
		return
	}

	// Print type information unless already handled elsewhere.
	if !d.ignoreNextType {
		d.indent()
		d.w.Write(openParenBytes)
		io.WriteString(d.w, v.Type().String())
		d.w.Write(closeParenBytes)
		d.w.Write(spaceBytes)
	}
	d.ignoreNextType = false

	// Display length and capacity if the built-in len and cap functions
	// work with the value's kind and the len/cap itself is non-zero.
	valueLen, valueCap := 0, 0
	switch v.Kind() {
	case reflect.Array, reflect.Slice, reflect.Chan:
		valueLen, valueCap = v.Len(), v.Cap()
	case reflect.Map, reflect.String:
		valueLen = v.Len()
	}
	if valueLen != 0 || !d.cs.DisableCapacities && valueCap != 0 {
		d.w.Write(openParenBytes)
		if valueLen != 0 {
			d.w.Write(lenEqualsBytes)
			printInt(d.w, int64(valueLen), 10)
		}
		if !d.cs.DisableCapacities && valueCap != 0 {
			if valueLen != 0 {
				d.w.Write(spaceBytes)
			}
			d.w.Write(capEqualsBytes)
			printInt(d.w, int64(valueCap), 10)
		}
		d.w.Write(closeParenBytes)
		d.w.Write(spaceBytes)
	}

	printValue(d.w, d, v, kind, d.cs)
}

func (d *dumpState) printArray(v reflect.Value) {
	d.w.Write(openBraceNewlineBytes)
	d.depth++
	if (d.cs.MaxDepth != 0) && (d.depth > d.cs.MaxDepth) {
		d.indent()
		d.w.Write(maxNewlineBytes)
	} else {
		d.dumpSlice(v)
	}
	d.depth--
	d.indent()
	d.w.Write(closeBraceBytes)
}

func (d *dumpState) printString(v reflect.Value) {
	b := bufferGet()
	defer bufferPut(b)
	b.SetBytes(strconv.AppendQuote(b.Bytes(), v.String()))
	d.w.Write(b.Bytes())
}

func (d *dumpState) printMap(v reflect.Value) {
	d.w.Write(openBraceNewlineBytes)
	d.depth++
	if (d.cs.MaxDepth != 0) && (d.depth > d.cs.MaxDepth) {
		d.indent()
		d.w.Write(maxNewlineBytes)
	} else {
		numEntries := v.Len()
		keys := v.MapKeys()
		if d.cs.SortKeys {
			sortValues(keys, d.cs)
		}
		for i, key := range keys {
			d.dump(d.unpackValue(key))
			d.w.Write(colonSpaceBytes)
			d.ignoreNextIndent = true
			d.dump(d.unpackValue(v.MapIndex(key)))
			if i < (numEntries - 1) {
				d.w.Write(commaNewlineBytes)
			} else {
				d.w.Write(newlineBytes)
			}
		}
	}
	d.depth--
	d.indent()
	d.w.Write(closeBraceBytes)
}

func (d *dumpState) printStruct(v reflect.Value) {
	d.w.Write(openBraceNewlineBytes)
	d.depth++
	if (d.cs.MaxDepth != 0) && (d.depth > d.cs.MaxDepth) {
		d.indent()
		d.w.Write(maxNewlineBytes)
	} else {
		vt := v.Type()
		numFields := v.NumField()
		for i := 0; i < numFields; i++ {
			d.indent()
			vtf := vt.Field(i)
			io.WriteString(d.w, vtf.Name)
			d.w.Write(colonSpaceBytes)
			d.ignoreNextIndent = true
			d.dump(d.unpackValue(v.Field(i)))
			if i < (numFields - 1) {
				d.w.Write(commaNewlineBytes)
			} else {
				d.w.Write(newlineBytes)
			}
		}
	}
	d.depth--
	d.indent()
	d.w.Write(closeBraceBytes)
}

func (d *dumpState) defaultFormat() string {
	return "%v"
}

func (d *dumpState) Reset(w io.Writer, cs *ConfigState) {
	*d = dumpState{w: w, ci: d.ci, cs: cs}
}

// fdump is a helper function to consolidate the logic from the various public
// methods which take varying writers and config states.
func fdump(cs *ConfigState, w io.Writer, a ...interface{}) {
	d := dumpStateGet(w, cs)
	defer dumpStatePut(d)

	for _, arg := range a {
		if arg == nil {
			w.Write(interfaceBytes)
			w.Write(spaceBytes)
			w.Write(nilAngleBytes)
			w.Write(newlineBytes)
			continue
		}

		for k := range d.ci.pointers {
			delete(d.ci.pointers, k)
		}

		d.dump(reflect.ValueOf(arg))
		d.w.Write(newlineBytes)
	}
}

var dumpStatePool = sync.Pool{New: func() interface{} {
	return new(dumpState)
}}

func dumpStateGet(w io.Writer, cs *ConfigState) *dumpState {
	d := dumpStatePool.Get().(*dumpState)
	d.ci = cycleInfoGet()
	d.Reset(w, cs)
	return d
}

func dumpStatePut(d *dumpState) {
	cycleInfoPut(d.ci)
	d.ci = nil
	dumpStatePool.Put(d)
}
