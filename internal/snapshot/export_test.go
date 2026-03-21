package snapshot

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Meru143/lantern/internal/store"
	"github.com/stretchr/testify/require"
)

func TestExportCreatesFilteredBundle(t *testing.T) {
	t.Parallel()

	sourceDB := openSnapshotDB(t, t.TempDir()+"/source.db")
	seedSnapshotData(t, sourceDB)

	outPath := filepath.Join(t.TempDir(), "fixture.lantern")
	err := Export(context.Background(), dbPath(t, sourceDB), time.Minute, outPath)
	require.NoError(t, err)

	archive, err := zip.OpenReader(outPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = archive.Close()
	})

	require.Len(t, archive.File, 2)

	metaFile := findArchiveFile(t, archive.File, "meta.json")
	metaReader, err := metaFile.Open()
	require.NoError(t, err)
	defer metaReader.Close()

	var meta ExportMetadata
	require.NoError(t, json.NewDecoder(metaReader).Decode(&meta))
	require.Equal(t, "1", meta.Version)
	require.Equal(t, int64(60), meta.DurationSeconds)
	require.Equal(t, 1, meta.SpansCount)
	require.Equal(t, 1, meta.LogsCount)

	dataFile := findArchiveFile(t, archive.File, "data.db")
	dataReader, err := dataFile.Open()
	require.NoError(t, err)
	defer dataReader.Close()

	extractedDBPath := filepath.Join(t.TempDir(), "extracted.db")
	extractedFile, err := os.Create(extractedDBPath)
	require.NoError(t, err)
	_, err = io.Copy(extractedFile, dataReader)
	require.NoError(t, err)
	require.NoError(t, extractedFile.Close())

	exportedDB := openSnapshotDB(t, extractedDBPath)
	traces, err := store.ListTraces(context.Background(), exportedDB, store.TraceFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, traces, 1)
	require.Equal(t, "trace-recent", traces[0].TraceID)
}

func TestImportRestoresBundle(t *testing.T) {
	t.Parallel()

	sourceDB := openSnapshotDB(t, t.TempDir()+"/source.db")
	seedSnapshotData(t, sourceDB)

	bundlePath := filepath.Join(t.TempDir(), "fixture.lantern")
	require.NoError(t, Export(context.Background(), dbPath(t, sourceDB), time.Minute, bundlePath))

	targetPath := filepath.Join(t.TempDir(), "restored.db")
	require.NoError(t, Import(bundlePath, targetPath))

	targetDB := openSnapshotDB(t, targetPath)
	traces, err := store.ListTraces(context.Background(), targetDB, store.TraceFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, traces, 1)
	require.Equal(t, "trace-recent", traces[0].TraceID)
}

func openSnapshotDB(t *testing.T, path string) *sql.DB {
	t.Helper()

	db, err := store.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})
	require.NoError(t, store.RunMigrations(db))
	return db
}

func seedSnapshotData(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	now := time.Now()
	oldTime := now.Add(-2 * time.Hour).UnixNano()
	recentTime := now.Add(-30 * time.Second).UnixNano()

	oldSpan := store.SpanRecord{
		TraceID:       "trace-old",
		SpanID:        "span-old",
		ServiceName:   "frontend",
		Name:          "GET /old",
		StartTimeUnix: oldTime,
		EndTimeUnix:   oldTime + int64(5*time.Millisecond),
		Attributes:    `{"http.method":"GET"}`,
		Events:        `[]`,
	}
	recentSpan := store.SpanRecord{
		TraceID:       "trace-recent",
		SpanID:        "span-recent",
		ServiceName:   "frontend",
		Name:          "GET /recent",
		StartTimeUnix: recentTime,
		EndTimeUnix:   recentTime + int64(10*time.Millisecond),
		Attributes:    `{"db.system":"postgresql","db.statement":"SELECT * FROM users WHERE id = 123"}`,
		Events:        `[{"name":"exception","attributes":{"exception.type":"ValueError","exception.message":"recent failure 123"}}]`,
	}

	require.NoError(t, store.InsertSpan(ctx, db, oldSpan))
	require.NoError(t, store.InsertSpan(ctx, db, recentSpan))
	require.NoError(t, store.RecordDBSpan(ctx, db, recentSpan))

	require.NoError(t, store.InsertLog(ctx, db, store.LogRecord{
		TraceID:      "trace-old",
		SpanID:       "span-old",
		ServiceName:  "frontend",
		Timestamp:    oldTime + int64(time.Millisecond),
		SeverityNum:  9,
		SeverityText: "INFO",
		Body:         "old log",
		Attributes:   `{"component":"old"}`,
	}))
	require.NoError(t, store.InsertLog(ctx, db, store.LogRecord{
		TraceID:      "trace-recent",
		SpanID:       "span-recent",
		ServiceName:  "frontend",
		Timestamp:    recentTime + int64(time.Millisecond),
		SeverityNum:  17,
		SeverityText: "ERROR",
		Body:         "recent log",
		Attributes:   `{"component":"recent"}`,
	}))
}

func findArchiveFile(t *testing.T, files []*zip.File, name string) *zip.File {
	t.Helper()

	for _, file := range files {
		if file.Name == name {
			return file
		}
	}
	t.Fatalf("archive entry %s not found", name)
	return nil
}

func dbPath(t *testing.T, db *sql.DB) string {
	t.Helper()

	var path string
	err := db.QueryRow(`PRAGMA database_list`).Scan(new(interface{}), new(interface{}), &path)
	require.NoError(t, err)
	return path
}
