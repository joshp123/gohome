package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
)

type outputMode struct {
	json bool
}

func (o outputMode) printJSON(value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatal("format json", err)
	}
	fmt.Println(string(data))
}

func (o outputMode) table(rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	for _, row := range rows {
		fmt.Fprintln(w, joinRow(row))
	}
	_ = w.Flush()
}

func joinRow(row []string) string {
	if len(row) == 0 {
		return ""
	}
	out := row[0]
	for i := 1; i < len(row); i++ {
		out += "\t" + row[i]
	}
	return out
}
