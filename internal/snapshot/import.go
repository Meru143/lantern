package snapshot

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Import restores a .lantern bundle to the provided target database path.
func Import(bundlePath string, targetDBPath string) error {
	archive, err := zip.OpenReader(bundlePath)
	if err != nil {
		return wrapSnapshotIOError(bundlePath, err)
	}
	defer archive.Close()

	metaFile, dataFile, err := findBundleFiles(archive.File)
	if err != nil {
		return err
	}

	metaReader, err := metaFile.Open()
	if err != nil {
		return wrapSnapshotIOError(bundlePath, err)
	}
	defer metaReader.Close()

	var meta ExportMetadata
	if err := json.NewDecoder(metaReader).Decode(&meta); err != nil {
		return wrapSnapshotIOError(bundlePath, err)
	}
	if meta.Version != "1" {
		return fmt.Errorf("unsupported snapshot version %q", meta.Version)
	}

	if err := os.MkdirAll(filepath.Dir(targetDBPath), 0o755); err != nil {
		return wrapSnapshotIOError(targetDBPath, err)
	}

	tempTarget := targetDBPath + ".tmp"
	if err := writeArchiveFile(dataFile, tempTarget); err != nil {
		return err
	}
	defer os.Remove(tempTarget)

	if err := os.RemoveAll(targetDBPath); err != nil {
		return wrapSnapshotIOError(targetDBPath, err)
	}
	if err := os.Rename(tempTarget, targetDBPath); err != nil {
		return wrapSnapshotIOError(targetDBPath, err)
	}

	return nil
}

func findBundleFiles(files []*zip.File) (*zip.File, *zip.File, error) {
	var (
		metaFile *zip.File
		dataFile *zip.File
	)

	for _, file := range files {
		switch file.Name {
		case "meta.json":
			metaFile = file
		case "data.db":
			dataFile = file
		}
	}

	if metaFile == nil {
		return nil, nil, fmt.Errorf("meta.json not found in bundle")
	}
	if dataFile == nil {
		return nil, nil, fmt.Errorf("data.db not found in bundle")
	}

	return metaFile, dataFile, nil
}

func writeArchiveFile(file *zip.File, targetPath string) error {
	reader, err := file.Open()
	if err != nil {
		return wrapSnapshotIOError(targetPath, err)
	}
	defer reader.Close()

	writer, err := os.Create(targetPath)
	if err != nil {
		return wrapSnapshotIOError(targetPath, err)
	}
	defer writer.Close()

	if _, err := io.Copy(writer, reader); err != nil {
		return wrapSnapshotIOError(targetPath, err)
	}

	return nil
}
