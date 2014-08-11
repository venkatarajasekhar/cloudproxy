// Copyright (c) 2014, Kevin Walsh.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

// This file implements Scan() functions for all elements so they can be used
// with fmt.Scanf() and friends.

import (
	"fmt"
)

// Scan parses a Prin, with optional outer parens.
func (p *Prin) Scan(state fmt.ScanState, verb rune) error {
	parser := inputParser(state)
	prin, err := parser.parsePrin()
	if err != nil {
		return err
	}
	*p = prin
	return nil
}

// Scan parses a PrinExt.
func (e *PrinExt) Scan(state fmt.ScanState, verb rune) error {
	parser := inputParser(state)
	name, args, err := parser.expectNameAndArgs()
	if err != nil {
		return err
	}
	e.Name = name
	e.Args = args
	return nil
}

// Scan parses a Term, with optional outer parens.
func (t *Term) Scan(state fmt.ScanState, verb rune) error {
	parser := inputParser(state)
	term, err := parser.parseTerm()
	if err != nil {
		return err
	}
	*t = term
}

// Scan parses a String, with optional outer parens.
func (t *String) Scan(state fmt.ScanState, verb rune) error {
	parser := inputParser(state)
	s, err := parser.parseString()
	if err != nil {
		return err
	}
	*t = s
	return nil
}

// Scan parses an Int, with optional outer parens.
func (t *Int) Scan(state fmt.ScanState, verb rune) error {
	parser := inputParser(state)
	i, err := parser.parseInt()
	if err != nil {
		return err
	}
	*t = i
	return nil
}

// Scan parses a Form, with optional outer parens. This function is not greedy:
// it consumes only as much input as necessary to obtain a valid formula. For
// example, "(p says a and b ...)" and "p says (a and b ...) will be parsed in
// their entirety, but given "p says a and b ... ", only "p says a" will be
// parsed.
func (f *Form) Scan(state fmt.ScanState, verb rune) error {
	parser := inputParser(state)
	form, err := parser.parseShortestForm()
	if err != nil {
		return err
	}
	*f = form
	return nil
}

// Scan parses a Pred, with optional outer parens.
func (f *Pred) Scan(state fmt.ScanState, verb rune) error {
	parser := inputParser(state)
	pred, err := parser.parsePred()
	if err != nil {
		return err
	}
	*f = pred
	return nil
}

// Scan parses a Const, with optional outer parens. This function is not greedy.
func (f *Const) Scan(state fmt.ScanState, verb rune) error {
	parser := inputParser(state)
	c, err := parser.parseConst()
	if err != nil {
		return err
	}
	*f = c
	return nil
}

// Scan parses a Not, with optional outer parens. This function is not greedy.
func (f *Not) Scan(state fmt.ScanState, verb rune) error {
	form, err := parser.parseShortestForm()
	if err != nil {
		return err
	}
	n, ok := form.(And)
	if !ok {
		return fmt.Errorf(`expecting "and": %s`, form)
	}
	*f = n
	return nil
}

// Scan parses an And, with required outer parens. This function is not greedy.
// BUG(kwalsh): This won't succeed unless there are outer parens. For
// consistency, perhaps I need to make non-greedy parse functions for each
// operator?
func (f *And) Scan(state fmt.ScanState, verb rune) error {
	form, err := parser.parseShortestForm()
	if err != nil {
		return err
	}
	n, ok := form.(And)
	if !ok {
		return fmt.Errorf(`expecting "and": %s`, form)
	}
	*f = n
	return nil
}

// Scan parses an Or, with required outer parens. This function is not greedy.
// BUG(kwalsh): This won't succeed unless there are outer parens. For
// consistency, perhaps I need to make non-greedy parse functions for each
// operator?
func (f *Or) Scan(state fmt.ScanState, verb rune) error {
	form, err := parser.parseShortestForm()
	if err != nil {
		return err
	}
	n, ok := form.(Or)
	if !ok {
		return fmt.Errorf(`expecting "or": %s`, form)
	}
	*f = n
	return nil
}

// Scan parses an Implies, with required outer parens. This function is not
// greedy.
// BUG(kwalsh): This won't succeed unless there are outer parens. For
// consistency, perhaps I need to make non-greedy parse functions for each
// operator?
func (f *Implies) Scan(state fmt.ScanState, verb rune) error {
	form, err := parser.parseShortestForm()
	if err != nil {
		return err
	}
	n, ok := form.(Implies)
	if !ok {
		return fmt.Errorf(`expecting "implies": %s`, form)
	}
	*f = n
	return nil
}

// Scan parses a Says, with optional outer parens. This function is not greedy.
func (f *Says) Scan(state fmt.ScanState, verb rune) error {
	form, err := parser.parseShortestForm()
	if err != nil {
		return err
	}
	n, ok := form.(Says)
	if !ok {
		return fmt.Errorf(`expecting "says": %s`, form)
	}
	*f = n
	return nil
}

// Scan parses a Speaksfor, with optional outer parens. This function is not
// greedy.
func (f *Speaksfor) Scan(state fmt.ScanState, verb rune) error {
	form, err := parser.parseShortestForm()
	if err != nil {
		return err
	}
	n, ok := form.(Speaksfor)
	if !ok {
		return fmt.Errorf(`expecting "speaksfor": %s`, form)
	}
	*f = n
	return nil
}

