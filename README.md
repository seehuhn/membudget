# seehuhn.de/go/membudget

[![Go Reference](https://pkg.go.dev/badge/seehuhn.de/go/membudget.svg)](https://pkg.go.dev/seehuhn.de/go/membudget)
[![Go Report Card](https://goreportcard.com/badge/seehuhn.de/go/membudget)](https://goreportcard.com/report/seehuhn.de/go/membudget)

A small Go package providing cumulative memory budgets for parsing
untrusted input.  A `Budget` is sized at construction, decremented by
each allocation, and discarded when the parse completes; the budget
never grants memory back, so it bounds peak memory at most by the total
it was sized for.

Copyright (C) 2026 Jochen Voss

This program is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
