package processor

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/personalmedia/cdn/internal/cache"
	"github.com/xuri/excelize/v2"
)

func HandleExcelCSV(c *gin.Context, req *ActionRequest) {
	cacheFile := cache.CacheFileForDerived("csv", req.RelPath, ".csv")
	filename := strings.TrimSuffix(filepath.Base(req.RelPath), filepath.Ext(req.RelPath)) + ".csv"

	c.Header(
		"Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename)),
	)

	cache.GenerateCached(c, cacheFile, "text/csv; charset=utf-8", []byte(""), func() ([]byte, error) {
		f, err := excelize.OpenFile(req.SourceFile, excelize.Options{UnzipXMLSizeLimit: 250 * 1024 * 1024})
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()

		sheet := primarySheetName(f)
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, err
		}

		buf := new(bytes.Buffer)
		writer := csv.NewWriter(buf)

		for _, row := range rows {
			if err := writer.Write(row); err != nil {
				return nil, err
			}
		}

		writer.Flush()
		if err := writer.Error(); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	})
}

func HandleExcelJSON(c *gin.Context, req *ActionRequest) {
	cacheFile := cache.CacheFileForDerived("json", req.RelPath, ".json")
	filename := strings.TrimSuffix(filepath.Base(req.RelPath), filepath.Ext(req.RelPath)) + ".json"

	c.Header(
		"Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename)),
	)

	cache.GenerateCached(c, cacheFile, "application/json; charset=utf-8", []byte("[]"), func() ([]byte, error) {
		f, err := excelize.OpenFile(req.SourceFile, excelize.Options{UnzipXMLSizeLimit: 250 * 1024 * 1024})
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()

		sheets := f.GetSheetList()
		payload := make(map[string][][]string, len(sheets))

		for _, sheet := range sheets {
			rows, err := f.GetRows(sheet)
			if err != nil {
				return nil, err
			}
			payload[sheet] = rows
		}

		return json.Marshal(payload)
	})
}

func primarySheetName(f *excelize.File) string {
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return "Sheet1"
	}
	return sheets[0]
}
