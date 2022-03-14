// Copyright 2021-2022 Matthew R. Wilson <mwilson@mattwilson.org>
//
// This file is part of virtual1403
// <https://github.com/racingmars/virtual1403>.
//
// virtual1403 is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// virtual1403 is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with virtual1403. If not, see <https://www.gnu.org/licenses/>.

package scanner

// PrinterHandler interface receives the output of printer output parsing.
type PrinterHandler interface {
	AddLine(line string, linefeed bool)
	PageBreak()
	EndOfJob(jobinfo string)
}

const maxLineLen = 132

const (
	charTab byte = 0x9
	charLF  byte = 0xA
	charFF  byte = 0xC
	charCR  byte = 0xD
)
