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

import "io"

// Errorf is a wrapper for fmt.Errorf that treats each argument as if it were
// passed with a default Formatter interface returned by NewFormatter.  It
// returns the formatted string as a value that satisfies error.  See
// NewFormatter for formatting details.
//
// This function is shorthand for the following syntax:
//
//	fmt.Errorf(format, spew.NewFormatter(a), spew.NewFormatter(b))
func Errorf(format string, a ...interface{}) (err error) {
	return Config.Errorf(format, a...)
}

// Fprint is a wrapper for fmt.Fprint that treats each argument as if it were
// passed with a default Formatter interface returned by NewFormatter.  It
// returns the number of bytes written and any write error encountered.  See
// NewFormatter for formatting details.
//
// This function is shorthand for the following syntax:
//
//	fmt.Fprint(w, spew.NewFormatter(a), spew.NewFormatter(b))
func Fprint(w io.Writer, a ...interface{}) (n int, err error) {
	return Config.Fprint(w, a...)
}

// Fprintf is a wrapper for fmt.Fprintf that treats each argument as if it were
// passed with a default Formatter interface returned by NewFormatter.  It
// returns the number of bytes written and any write error encountered.  See
// NewFormatter for formatting details.
//
// This function is shorthand for the following syntax:
//
//	fmt.Fprintf(w, format, spew.NewFormatter(a), spew.NewFormatter(b))
func Fprintf(w io.Writer, format string, a ...interface{}) (n int, err error) {
	return Config.Fprintf(w, format, a...)
}

// Fprintln is a wrapper for fmt.Fprintln that treats each argument as if it
// passed with a default Formatter interface returned by NewFormatter.  See
// NewFormatter for formatting details.
//
// This function is shorthand for the following syntax:
//
//	fmt.Fprintln(w, spew.NewFormatter(a), spew.NewFormatter(b))
func Fprintln(w io.Writer, a ...interface{}) (n int, err error) {
	return Config.Fprintln(w, a...)
}

// Print is a wrapper for fmt.Print that treats each argument as if it were
// passed with a default Formatter interface returned by NewFormatter.  It
// returns the number of bytes written and any write error encountered.  See
// NewFormatter for formatting details.
//
// This function is shorthand for the following syntax:
//
//	fmt.Print(spew.NewFormatter(a), spew.NewFormatter(b))
func Print(a ...interface{}) (n int, err error) {
	return Config.Print(a...)
}

// Printf is a wrapper for fmt.Printf that treats each argument as if it were
// passed with a default Formatter interface returned by NewFormatter.  It
// returns the number of bytes written and any write error encountered.  See
// NewFormatter for formatting details.
//
// This function is shorthand for the following syntax:
//
//	fmt.Printf(format, spew.NewFormatter(a), spew.NewFormatter(b))
func Printf(format string, a ...interface{}) (n int, err error) {
	return Config.Printf(format, a...)
}

// Println is a wrapper for fmt.Println that treats each argument as if it were
// passed with a default Formatter interface returned by NewFormatter.  It
// returns the number of bytes written and any write error encountered.  See
// NewFormatter for formatting details.
//
// This function is shorthand for the following syntax:
//
//	fmt.Println(spew.NewFormatter(a), spew.NewFormatter(b))
func Println(a ...interface{}) (n int, err error) {
	return Config.Println(a...)
}

// Sprint is a wrapper for fmt.Sprint that treats each argument as if it were
// passed with a default Formatter interface returned by NewFormatter.  It
// returns the resulting string.  See NewFormatter for formatting details.
//
// This function is shorthand for the following syntax:
//
//	fmt.Sprint(spew.NewFormatter(a), spew.NewFormatter(b))
func Sprint(a ...interface{}) string {
	return Config.Sprint(a...)
}

// Sprintf is a wrapper for fmt.Sprintf that treats each argument as if it were
// passed with a default Formatter interface returned by NewFormatter.  It
// returns the resulting string.  See NewFormatter for formatting details.
//
// This function is shorthand for the following syntax:
//
//	fmt.Sprintf(format, spew.NewFormatter(a), spew.NewFormatter(b))
func Sprintf(format string, a ...interface{}) string {
	return Config.Sprintf(format, a...)
}

// Sprintln is a wrapper for fmt.Sprintln that treats each argument as if it
// were passed with a default Formatter interface returned by NewFormatter.  It
// returns the resulting string.  See NewFormatter for formatting details.
//
// This function is shorthand for the following syntax:
//
//	fmt.Sprintln(spew.NewFormatter(a), spew.NewFormatter(b))
func Sprintln(a ...interface{}) string {
	return Config.Sprintln(a...)
}

// Fdump formats and displays the passed arguments to io.Writer w.  It formats
// exactly the same as Dump.
func Fdump(w io.Writer, a ...interface{}) {
	Config.Fdump(w, a...)
}

// Sdump returns a string with the passed arguments formatted exactly the same
// as Dump.
func Sdump(a ...interface{}) string {
	return Config.Sdump(a...)
}

/*
Dump displays the passed parameters to standard out with newlines, customizable
indentation, and additional debug information such as complete types and all
pointer addresses used to indirect to the final value.  It provides the
following features over the built-in printing facilities provided by the fmt
package:

	* Pointers are dereferenced and followed
	* Circular data structures are detected and handled properly
	* Custom Stringer/error interfaces are optionally invoked, including
	  on unexported types
	* Custom types which only implement the Stringer/error interfaces via
	  a pointer receiver are optionally invoked when passing non-pointer
	  variables
	* Byte arrays and slices are dumped like the hexdump -C command which
	  includes offsets, byte values in hex, and ASCII output

The configuration options are controlled by an exported package global,
spew.Config.  See ConfigState for options documentation.

See Fdump if you would prefer dumping to an arbitrary io.Writer or Sdump to
get the formatted result as a string.
*/
func Dump(a ...interface{}) {
	Config.Dump(a...)
}
