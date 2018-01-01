package main

import (
	"fmt"
	"log"
	"os"

	"github.com/rn/iso9660wrap"
)

func printUsage() {
	fmt.Fprintf(os.Stderr, "usage: %s INFILE OUTFILE\n", os.Args[0])
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--help" {
		printUsage()
		os.Exit(0)
	} else if len(os.Args) != 3 {
		printUsage()
		os.Exit(1)
	}

	log.SetFlags(0)

	infile := os.Args[1]
	outfile := os.Args[2]

	outfh, err := os.OpenFile(outfile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("could not open output file %s for writing: %s", outfile, err)
	}
	infh, err := os.Open(infile)
	if err != nil {
		log.Fatalf("could not open input file %s for reading: %s", infile, err)
	}

	err = iso9660wrap.WriteFile(outfh, infh)
	if err != nil {
		log.Fatalf("writing file failed with %s", err)
	}
}
