package iso9660wrap

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"
)

func Panicf(format string, v ...interface{}) {
	panic(fmt.Errorf(format, v...))
}

const volumeDescriptorSetMagic = "\x43\x44\x30\x30\x31\x01" // Identifier = CD001, Version 0x01

const primaryVolumeSectorNum uint32 = 16
const numVolumeSectors uint32 = 2 // primary + terminator
const littleEndianPathTableSectorNum uint32 = primaryVolumeSectorNum + numVolumeSectors
const bigEndianPathTableSectorNum uint32 = littleEndianPathTableSectorNum + 1
const numPathTableSectors = 2 // no secondaries
const rootDirectorySectorNum uint32 = primaryVolumeSectorNum + numVolumeSectors + numPathTableSectors

// WriteFile writes the contents of infh to an iso at outfh with the name provided
func WriteFile(outfh, infh *os.File) error {
	fileSize, filename, err := getInputFileSizeAndName(infh)
	if err != nil {
		return err
	}
	filename = strings.ToUpper(filename)
	if !filenameSatisfiesISOConstraints(filename) {
		return fmt.Errorf("Input file name %s does not satisfy the ISO9660 character set constraints", filename)
	}

	buf := make([]byte, fileSize, fileSize)
	_, err = infh.Read(buf)
	if err != nil {
		return err
	}

	return WriteBuffer(outfh, buf, filename)
}

type FileEntry struct {
	File     *os.File
	Filename string
	Size     uint32
	Lba      uint32
}

// WriteFiles
func WriteFiles(outfile string, infiles []string) error {

	filelist := []FileEntry{}

	// Open all input files to check access, get size and validate filenames for ISO9660 compliance
	for _, inFilename := range infiles {
		// Will canonicalize path, check access
		inFileh, err := os.Open(inFilename)
		if err != nil {
			return fmt.Errorf("could not open input file %s for reading: %s", inFilename, err)
		}

		fileSize, filename, err := getInputFileSizeAndName(inFileh)
		if err != nil {
			return err
		}

		filename = strings.ToUpper(filename)
		if !filenameSatisfiesISOConstraints(filename) {
			return fmt.Errorf("Input file name %s does not satisfy the ISO9660 character set constraints", filename)
		}

		filelist = append(filelist, FileEntry{inFileh, filename, fileSize, 0})
	}

	// TODO - need to sort directories and filenames in correct order

	// Build a running list of lbas and total sizes used by files
	totalfilesize := uint32(0)
	currentlba := rootDirectorySectorNum + 1 // Would need to change if size of directory + file entries exceeds a sector
	for i := range filelist {
		// Update running total of filesizes and lbas used
		totalfilesize = totalfilesize + filelist[i].Size
		(&filelist[i]).Lba = currentlba
		currentlba = currentlba + numDataSectors(filelist[i].Size)
	}

	// Open output file
	outfh, err := os.OpenFile(outfile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("could not open output file %s for writing: %s", outfile, err)
	}

	// This is going to run all in ram, so don't make any huge ISO files yet

	// reserved sectors
	reservedAreaLength := int64(16 * SectorSize)
	_, err = outfh.Write(make([]byte, reservedAreaLength))
	if err != nil {
		return fmt.Errorf("could not write to output file: %s", err)
	}

	// writer for rest
	bufw := bufio.NewWriter(outfh)

	w := NewISO9660Writer(bufw)

	// TODO should totalfilesize include sector padding per file, path tables, file/directory descriptors, ...?
	writePrimaryVolumeDescriptor(w, totalfilesize, "iso9660wrapped")
	writeVolumeDescriptorSetTerminator(w)

	// This path table only contains a root directory. Would need to change later if multiple directories needed
	writePathTable(w, binary.LittleEndian)
	writePathTable(w, binary.BigEndian)

	// Write out root directory, self reference and list of files
	// SectorWriter.Write() will panic if this exceeds a sector
	sw := w.NextSector()
	if w.CurrentSector() != rootDirectorySectorNum {
		Panicf("internal error: unexpected root directory sector %d", w.CurrentSector())
	}
	WriteDirectoryRecord(sw, "\x00", w.CurrentSector())
	WriteDirectoryRecord(sw, "\x01", rootDirectorySectorNum)
	for _, currentfile := range filelist {
		fmt.Printf("file %s at sector %d", currentfile.Filename, currentfile.Lba)
		WriteFileRecordHeader(sw, currentfile.Filename, currentfile.Lba, currentfile.Size)
	}

	// In a full implementation, this should be a recursive strategy following directories & files,
	// while checking max depth and concatenated path length limits. This is a simple implementation
	// putting all files in the root.
	for _, currentfile := range filelist {
		writeData(w, currentfile.File, currentfile.Size, currentfile.Filename)
	}

	w.Finish()

	err = bufw.Flush()
	if err != nil {
		panic(err)
	}

	//return fmt.Errorf("WriteFiles is still a work in progress")
	return nil
}

// WriteBuffer writes the contents of buf to an iso at outfh with the name provided
func WriteBuffer(outfh io.Writer, buf []byte, filename string) error {
	fileSize := uint32(len(buf))
	r := bytes.NewReader(buf)

	// reserved sectors
	reservedAreaLength := int64(16 * SectorSize)
	_, err := outfh.Write(make([]byte, reservedAreaLength))
	if err != nil {
		return fmt.Errorf("could not write to output file: %s", err)
	}

	err = nil
	func() {
		defer func() {
			var ok bool
			e := recover()
			if e != nil {
				err, ok = e.(error)
				if !ok {
					panic(e)
				}
			}
		}()

		bufw := bufio.NewWriter(outfh)

		w := NewISO9660Writer(bufw)

		writePrimaryVolumeDescriptor(w, fileSize, filename)
		writeVolumeDescriptorSetTerminator(w)
		writePathTable(w, binary.LittleEndian)
		writePathTable(w, binary.BigEndian)
		writeRootDirectoryRecord(w)
		writeData(w, r, fileSize, filename)
		if w.CurrentSector() != numTotalSectors(fileSize) {
			Panicf("internal error: unexpected last sector number (expected %d, actual %d)",
				numTotalSectors(fileSize), w.CurrentSector())
		}
		w.Finish()

		err := bufw.Flush()
		if err != nil {
			panic(err)
		}
	}()
	if err != nil {
		return fmt.Errorf("could not write to output file: %s", err)
	}
	return nil
}

func writePrimaryVolumeDescriptor(w *ISO9660Writer, fileSize uint32, filename string) {
	if len(filename) > 32 {
		filename = filename[:32]
	}
	now := time.Now()

	sw := w.NextSector()
	if w.CurrentSector() != primaryVolumeSectorNum {
		Panicf("internal error: unexpected primary volume sector %d", w.CurrentSector())
	}

	sw.WriteByte('\x01')
	sw.WriteString(volumeDescriptorSetMagic)
	sw.WriteByte('\x00')

	sw.WritePaddedString("", 32)       // system identifier
	sw.WritePaddedString(filename, 32) // volume identifier

	sw.WriteZeros(8)                                   // unused
	sw.WriteBothEndianDWord(numTotalSectors(fileSize)) // volume size in logical blocks
	sw.WriteZeros(32)                                  // unused

	sw.WriteBothEndianWord(1)                  // volume set size
	sw.WriteBothEndianWord(1)                  // volume sequence number
	sw.WriteBothEndianWord(uint16(SectorSize)) // logical block size
	sw.WriteBothEndianDWord(SectorSize)        // path table length - BUG this could vary past a certain number of directories

	sw.WriteLittleEndianDWord(littleEndianPathTableSectorNum)
	sw.WriteLittleEndianDWord(0) // no secondary path tables
	sw.WriteBigEndianDWord(bigEndianPathTableSectorNum)
	sw.WriteBigEndianDWord(0) // no secondary path tables

	WriteDirectoryRecord(sw, "\x00", rootDirectorySectorNum) // root directory

	sw.WritePaddedString("", 128) // volume set identifier
	sw.WritePaddedString("", 128) // publisher identifier
	sw.WritePaddedString("", 128) // data preparer identifier
	sw.WritePaddedString("", 128) // application identifier

	sw.WritePaddedString("", 37) // copyright file identifier
	sw.WritePaddedString("", 37) // abstract file identifier
	sw.WritePaddedString("", 37) // bibliographical file identifier

	sw.WriteDateTime(now)         // volume creation
	sw.WriteDateTime(now)         // most recent modification
	sw.WriteUnspecifiedDateTime() // expires
	sw.WriteUnspecifiedDateTime() // is effective (?)

	sw.WriteByte('\x01') // version
	sw.WriteByte('\x00') // reserved

	sw.PadWithZeros() // 512 (reserved for app) + 653 (future use)
}

func writeVolumeDescriptorSetTerminator(w *ISO9660Writer) {
	sw := w.NextSector()
	if w.CurrentSector() != primaryVolumeSectorNum+1 {
		Panicf("internal error: unexpected volume descriptor set terminator sector %d", w.CurrentSector())
	}

	sw.WriteByte('\xFF')
	sw.WriteString(volumeDescriptorSetMagic)

	sw.PadWithZeros()
}

func writePathTable(w *ISO9660Writer, bo binary.ByteOrder) {
	sw := w.NextSector()
	sw.WriteByte(1) // name length
	sw.WriteByte(0) // number of sectors in extended attribute record
	sw.WriteDWord(bo, rootDirectorySectorNum)
	sw.WriteWord(bo, 1) // parent directory recno (root directory)
	sw.WriteByte(0)     // identifier (root directory)
	sw.WriteByte(1)     // padding
	sw.PadWithZeros()
}

func writeRootDirectoryRecord(w *ISO9660Writer) {
	sw := w.NextSector()
	if w.CurrentSector() != rootDirectorySectorNum {
		Panicf("internal error: unexpected root directory sector %d", w.CurrentSector())
	}

	WriteDirectoryRecord(sw, "\x00", w.CurrentSector())
	WriteDirectoryRecord(sw, "\x01", rootDirectorySectorNum)
	// TODO - does this need to change with multiple files? probably
}

// Creates a single file record, then writes a file to it
// TODO Should rename to writeFile which would be more accurate
func writeData(w *ISO9660Writer, infh io.Reader, fileSize uint32, filename string) {
	startsector := w.CurrentSector()
	//WriteFileRecordHeader(sw, filename, w.CurrentSector()+1, fileSize)

	// Now stream the data.  Note that the first buffer is never of SectorSize,
	// since we've already filled a part of the sector.
	b := make([]byte, SectorSize)
	total := uint32(0)
	for {
		l, err := infh.Read(b)
		if err != nil && err != io.EOF {
			Panicf("could not read from input file: %s", err)
		}
		if l > 0 {
			sw := w.NextSector()
			sw.Write(b[:l])
			total += uint32(l)
		}
		if err == io.EOF {
			break
		}
	}
	if total != fileSize {
		Panicf("input file size changed while the ISO file was being created (expected to read %d, read %d)", fileSize, total)
	} else if w.CurrentSector() != numDataSectors(fileSize)+startsector {
		Panicf("internal error: unexpected last sector number (expected %d, actual %d)",
			numDataSectors(fileSize)+startsector, w.CurrentSector())
	}
}

func numDataSectors(fileSize uint32) uint32 {
	return (fileSize + (SectorSize - 1)) / SectorSize
}

func numTotalSectors(fileSize uint32) uint32 {
	return 1 + rootDirectorySectorNum + numDataSectors(fileSize)
}

func getInputFileSizeAndName(fh *os.File) (uint32, string, error) {
	fi, err := fh.Stat()
	if err != nil {
		return 0, "", err
	}
	if fi.Size() >= math.MaxUint32 {
		return 0, "", fmt.Errorf("file size %d is too large", fi.Size())
	}
	return uint32(fi.Size()), fi.Name(), nil
}

func filenameSatisfiesISOConstraints(filename string) bool {
	invalidCharacter := func(r rune) bool {
		// According to ISO9660, only capital letters, digits, and underscores
		// are permitted.  Some sources say a dot is allowed as well.  I'm too
		// lazy to figure it out right now.
		if r >= 'A' && r <= 'Z' {
			return false
		} else if r >= '0' && r <= '9' {
			return false
		} else if r == '_' {
			return false
		} else if r == '.' {
			return false
		}
		return true
	}
	return strings.IndexFunc(filename, invalidCharacter) == -1
}
