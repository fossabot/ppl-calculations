package models

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"github.com/sirupsen/logrus"
	"ppl-calculations/domain/export"
)

type Overview struct {
	Base

	Exports []ExportData
}

type ExportData struct {
	ID        string
	Name      string
	ViewUrl   string
	CreatedAt string
}

func OverviewFromExports(csrf string, ex []export.Export) interface{} {
	fs := Overview{
		Base: Base{
			Step: string(StepOverview),
			CSRF: csrf,
		},
		Exports: []ExportData{},
	}

	for _, e := range ex {
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		if err := enc.Encode(e); err != nil {
			logrus.WithError(err).Error("encoding export for view url")
			continue
		}

		fs.Exports = append(fs.Exports, ExportData{
			ID:        e.ID.String(),
			Name:      e.Name.String(),
			CreatedAt: e.CreatedAt.Format("15:04:05 02-01-2006"),
			ViewUrl:   fmt.Sprintf("/view?d=%s", base64.URLEncoding.EncodeToString(buf.Bytes())),
		})
	}

	return fs
}