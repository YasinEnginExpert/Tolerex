package main

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Statistics for a single client/operation combination
type OpStats struct {
	Count      int
	TotalBytes int64
	MinRTT     int64
	MaxRTT     int64
	SumRTT     int64
	Errors     int
	Mode       string
}

// Aggregated report data for HTML template
type ClientReport struct {
	ClientID   string
	Operations map[string]*OpStats
	TotalOps   int
	AvgRTT     float64
	Success    float64
	Throughput float64 // MB/s (rough estimate if timestamps available)
}

const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>Tolerex Stress Test Report</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background-color: #1a1a1a; color: #e0e0e0; margin: 40px; }
        .container { max-width: 1000px; margin: auto; background: #2d2d2d; padding: 25px; border-radius: 12px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
        h1 { color: #00d4ff; text-align: center; border-bottom: 2px solid #444; padding-bottom: 15px; }
        .summary-card { background: #3d3d3d; padding: 15px; margin-bottom: 20px; border-radius: 8px; border-left: 5px solid #00d4ff; }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; background: #333; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #444; }
        th { background-color: #444; color: #00d4ff; }
        .status-ok { color: #4caf50; font-weight: bold; }
        .status-err { color: #f44336; font-weight: bold; }
        .rtt-low { color: #4caf50; }
        .rtt-med { color: #ffeb3b; }
        .rtt-high { color: #f44336; }
        .client-section { margin-top: 40px; border-top: 1px dashed #666; padding-top: 20px; }
        .mode-tag { background: #555; padding: 2px 6px; border-radius: 4px; font-size: 0.85em; color: #aaa; }
    </style>
</head>
<body>
    <div class="container">
        <h1>TOLEREX PERFORMANCE REPORT</h1>
        
        {{range .}}
        <div class="client-section">
            <div class="summary-card">
                <h2>Client: <span style="color: #00d4ff;">{{.ClientID}}</span></h2>
                <p>Total Operations: <strong>{{.TotalOps}}</strong> | Success Rate: <span class="{{if ge .Success 99.0}}status-ok{{else}}status-err{{end}}">{{printf "%.2f" .Success}}%</span></p>
            </div>

            <table>
                <thead>
                    <tr>
                        <th>Operation</th>
                        <th>Mode</th>
                        <th>Count</th>
                        <th>Avg RTT (us)</th>
                        <th>Min/Max (us)</th>
                        <th>Data Sent</th>
                        <th>Errors</th>
                    </tr>
                </thead>
                <tbody>
                    {{range $opName, $stats := .Operations}}
                    {{$avg := 0.0}}{{if gt $stats.Count 0}}{{$avg = div $stats.SumRTT $stats.Count}}{{end}}
                    <tr>
                        <td><strong>{{$opName}}</strong></td>
                        <td><span class="mode-tag">{{$stats.Mode}}</span></td>
                        <td>{{$stats.Count}}</td>
                        <td class="{{if lt $avg 1000.0}}rtt-low{{else if lt $avg 5000.0}}rtt-med{{else}}rtt-high{{end}}">
                            {{printf "%.0f" $avg}}
                        </td>
                        <td style="font-size: 0.9em; color: #888;">{{$stats.MinRTT}} / {{$stats.MaxRTT}}</td>
                        <td>{{kb $stats.TotalBytes}} KB</td>
                        <td class="{{if gt $stats.Errors 0}}status-err{{else}}status-ok{{end}}">{{$stats.Errors}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{end}}

        <p style="text-align: center; margin-top: 50px; color: #666; font-size: 0.8em;">Generated at {{now}}</p>
    </div>
</body>
</html>
`

func main() {
	files, err := filepath.Glob("measured/*.csv")
	if err != nil || len(files) == 0 {
		fmt.Println("No CSV files found in measured/ directory.")
		return
	}

	reports := make([]*ClientReport, 0)

	for _, file := range files {
		report, err := processCSV(file)
		if err != nil {
			fmt.Printf("Error processing %s: %v\n", file, err)
			continue
		}
		reports = append(reports, report)
	}

	funcMap := template.FuncMap{
		"div": func(a, b int64) float64 { return float64(a) / float64(b) },
		"kb":  func(b int64) string { return fmt.Sprintf("%.2f", float64(b)/1024.0) },
		"now": func() string { return time.Now().Format(time.RFC1123) },
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		panic(err)
	}

	out, err := os.Create("measured/report.html")
	if err != nil {
		panic(err)
	}
	defer out.Close()

	err = tmpl.Execute(out, reports)
	if err != nil {
		panic(err)
	}

	fmt.Println("Successfully generated measured/report.html")
}

func processCSV(path string) (*ClientReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	// Skip header if it exists
	first, err := reader.Read()
	if err != nil {
		return nil, err
	}
	if len(first) > 0 && first[0] == "Timestamp" {
		// Definitely a header, proceed
	} else {
		// Not a header? Reset or process?
		// For simplicity, assume header exists. If not, this might miss the first row.
		// f.Seek(0, 0)
		// reader = csv.NewReader(f)
	}

	report := &ClientReport{
		Operations: make(map[string]*OpStats),
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Header: Timestamp,ClientID,Operation,Count,Bytes,RTT_us,Mode,Status,PayloadSize
		if len(record) < 9 {
			continue
		}

		clientID := record[1]
		opName := record[2]
		count, _ := strconv.Atoi(record[3])
		bytes, _ := strconv.ParseInt(record[4], 10, 64)
		rtt, _ := strconv.ParseInt(record[5], 10, 64)
		mode := record[6]
		status := record[7]

		report.ClientID = clientID

		stats, ok := report.Operations[opName]
		if !ok {
			stats = &OpStats{
				MinRTT: rtt,
				MaxRTT: rtt,
				Mode:   mode,
			}
			report.Operations[opName] = stats
		}

		stats.Count += count
		stats.TotalBytes += bytes
		stats.SumRTT += rtt
		if rtt < stats.MinRTT {
			stats.MinRTT = rtt
		}
		if rtt > stats.MaxRTT {
			stats.MaxRTT = rtt
		}
		if status == "ERROR" {
			stats.Errors += count
		}

		report.TotalOps += count
	}

	// Final calc
	var totalProcessed int64
	var totalErr int64
	for _, s := range report.Operations {
		totalProcessed += int64(s.Count)
		totalErr += int64(s.Errors)
	}

	if totalProcessed > 0 {
		report.Success = 100.0 - (float64(totalErr) / float64(totalProcessed) * 100.0)
	}

	return report, nil
}
