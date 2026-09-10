// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package customslc

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
)

var gzipMagic = []byte{0x1f, 0x8b}

func openTar(reader io.Reader) (*tar.Reader, func() error, error) {
	buffered := bufio.NewReader(reader)
	magic, err := buffered.Peek(len(gzipMagic))
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, nil, err
	}
	if errors.Is(err, io.EOF) || !bytes.Equal(magic, gzipMagic) {
		return tar.NewReader(buffered), func() error { return nil }, nil
	}

	gzipReader, err := gzip.NewReader(buffered)
	if err != nil {
		return nil, nil, err
	}
	finish := func() error {
		// Draining to EOF is what makes gzip verify its CRC; the size is unbounded by design
		// (a single-tenant, operator-supplied container of unpredictable size).
		//nolint:gosec // G110: unbounded decompression is intentional (see above).
		if _, err := io.Copy(io.Discard, gzipReader); err != nil {
			return err
		}

		return gzipReader.Close()
	}

	return tar.NewReader(gzipReader), finish, nil
}

type languageDefinitions struct {
	//nolint:tagliatelle // Metadata key is defined by the SLC archive format.
	LanguageDefinitions []languageDefinition `json:"language_definitions"`
}

type languageDefinition struct {
	Aliases []string `json:"aliases"`
}

// ReadAliases validates an SLC archive and returns the database aliases declared by it.
// The archive is streamed and is not extracted to disk.
func ReadAliases(reader io.Reader) ([]string, error) {
	tarReader, finish, err := openTar(reader)
	if err != nil {
		return nil, err
	}

	clientFound := false
	metadataFound := false
	var aliases []string
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if isClientExecutable(header) {
			clientFound = true
		}
		if path.Clean(header.Name) == "build_info/language_definitions.json" {
			if metadataFound {
				return nil, errors.New(
					"container archive contains duplicate " +
						"build_info/language_definitions.json",
				)
			}
			metadataFound = true
			parsed, err := readLanguageAliases(tarReader)
			if err != nil {
				return nil, err
			}
			aliases = append(aliases, parsed...)
		}
	}

	if err := finish(); err != nil {
		return nil, fmt.Errorf("container archive is corrupt: %w", err)
	}
	if !clientFound {
		return nil, fmt.Errorf(
			"%s was not found as an executable in the container; Exasol Personal supports "+
				"standard SLCs built with exaslct",
			clientRelPath,
		)
	}
	if !metadataFound || len(aliases) == 0 {
		return nil, errors.New("build_info/language_definitions.json contains no language aliases")
	}

	return aliases, nil
}

func readLanguageAliases(reader io.Reader) ([]string, error) {
	var definitions languageDefinitions
	if err := json.NewDecoder(reader).Decode(&definitions); err != nil {
		return nil, fmt.Errorf("failed to parse language definitions: %w", err)
	}

	var aliases []string
	for _, definition := range definitions.LanguageDefinitions {
		for _, alias := range definition.Aliases {
			normalized := NormalizeAlias(alias)
			if normalized == "" {
				return nil, errors.New("language definitions contain an empty alias")
			}
			if slices.Contains(aliases, normalized) {
				return nil, fmt.Errorf(
					"language definitions contain duplicate alias %q", normalized,
				)
			}
			aliases = append(aliases, normalized)
		}
	}

	return aliases, nil
}

func ValidateArchive(reader io.Reader) error {
	_, err := ReadAliases(reader)

	return err
}

func isClientExecutable(header *tar.Header) bool {
	if header.Typeflag != tar.TypeReg {
		return false
	}
	if path.Clean(header.Name) != clientRelPath {
		return false
	}

	return header.FileInfo().Mode().Perm()&0o111 != 0
}
