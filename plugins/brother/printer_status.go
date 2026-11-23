package brother

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// PrinterErrorEntry records one recent printer fault and the page counter when it occurred.
type PrinterErrorEntry struct {
	Message   string
	PageCount uint64
}

// PrinterStatus holds the strongly typed reading parsed from a Brother printer's
// /etc/mnt_info.csv maintenance page.
type PrinterStatus struct {
	NodeName               string
	ModelName              string
	IPAddress              string
	SerialNumber           string
	FirmwareVersion        string
	MemoryMegabytes        *uint32
	PageCounter            *uint64
	DuplexPageCounter      *uint64
	AverageCoveragePercent *float64
	DrumRemainingPercent   *float64
	TonerRemainingPercent  *float64
	TonerReplaceCount      *uint64
	DrumReplaceCount       *uint64
	TotalPaperJams         *uint64
	JamTray1Count          *uint64
	JamInsideCount         *uint64
	JamRearCount           *uint64
	JamDuplexCount         *uint64
	RecentErrors           []PrinterErrorEntry
	ObservedAt             time.Time
}

// ParseMaintenanceCSV decodes the two-row CSV served by Brother printers at /etc/mnt_info.csv.
func ParseMaintenanceCSV(raw io.Reader, observedAt time.Time) (PrinterStatus, error) {
	reader := csv.NewReader(raw)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	headers, err := reader.Read()
	if err != nil {
		return PrinterStatus{}, fmt.Errorf("read brother csv header: %w", err)
	}
	values, err := reader.Read()
	if err != nil {
		return PrinterStatus{}, fmt.Errorf("read brother csv row: %w", err)
	}

	columns := make(map[string]string, len(headers))
	for i, header := range headers {
		key := strings.TrimSpace(header)
		if key == "" || i >= len(values) {
			continue
		}
		if _, exists := columns[key]; !exists {
			columns[key] = strings.TrimSpace(values[i])
		}
	}

	status := PrinterStatus{
		NodeName:               columns["Node Name"],
		ModelName:              columns["Model Name"],
		IPAddress:              columns["IP Address"],
		SerialNumber:           columns["Serial No."],
		FirmwareVersion:        columns["Main Firmware Version"],
		MemoryMegabytes:        parseOptionalUint32(columns["Memory Size"]),
		PageCounter:            parseOptionalUint64(columns["Page Counter"]),
		DuplexPageCounter:      parseOptionalUint64(columns["Total 2-sided Print"]),
		AverageCoveragePercent: parseOptionalFloat64(columns["Average Coverage"]),
		DrumRemainingPercent:   parseOptionalFloat64(columns["% of Life Remaining(Drum Unit)"]),
		TonerRemainingPercent:  parseOptionalFloat64(columns["% of Life Remaining(Toner)"]),
		TonerReplaceCount:      parseOptionalUint64(columns["Replace Count(Toner)"]),
		DrumReplaceCount:       parseOptionalUint64(columns["Replace Count(Drum Unit)"]),
		TotalPaperJams:         parseOptionalUint64(columns["Total Paper Jams"]),
		JamTray1Count:          parseOptionalUint64(columns["Jam Tray 1"]),
		JamInsideCount:         parseOptionalUint64(columns["Jam Inside"]),
		JamRearCount:           parseOptionalUint64(columns["Jam Rear"]),
		JamDuplexCount:         parseOptionalUint64(columns["Jam 2-sided"]),
		ObservedAt:             observedAt,
	}

	for i := 1; i <= 10; i++ {
		msg := strings.TrimSpace(columns[fmt.Sprintf("Error%d", i)])
		if msg == "" {
			continue
		}
		var pageCount uint64
		if parsed := parseOptionalUint64(columns[fmt.Sprintf("Error Count %d", i)]); parsed != nil {
			pageCount = *parsed
		}
		status.RecentErrors = append(status.RecentErrors, PrinterErrorEntry{
			Message:   msg,
			PageCount: pageCount,
		})
	}

	if status.ModelName == "" && status.NodeName == "" && status.PageCounter == nil {
		return PrinterStatus{}, fmt.Errorf("brother csv contained no recognized printer fields")
	}

	return status, nil
}

func parseOptionalUint32(raw string) *uint32 {
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return nil
	}
	out := uint32(v)
	return &out
}

func parseOptionalUint64(raw string) *uint64 {
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseOptionalFloat64(raw string) *float64 {
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &v
}
