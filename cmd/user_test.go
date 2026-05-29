package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	importsync "codesome-usage-manager/internal/sync"
)

func TestPrintUserImportResults(t *testing.T) {
	var buf bytes.Buffer
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = originalStdout }()

	groupID := 51
	printUserImportResults([]importsync.ImportUsersResult{
		{
			Row:             2,
			Action:          "create",
			EmployeeNo:      "E12345",
			Name:            "Alice",
			TeamCode:        "platform",
			Status:          "active",
			CodesomeGroupID: &groupID,
		},
	})
	writer.Close()
	if _, err := buf.ReadFrom(reader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "ACTION") || !strings.Contains(got, "E12345") || !strings.Contains(got, "platform") {
		t.Fatalf("unexpected output: %s", got)
	}
}
