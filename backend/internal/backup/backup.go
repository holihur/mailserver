// Package backup 提供一键备份/恢复：DB 全表 JSON + DATA_DIR 目录打包为 tar.gz。
package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gorm.io/gorm"
)

// Create 导出所有表为 db.json，并连同 dataDir 打包到 out。
func Create(db *gorm.DB, dataDir, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	data, err := exportDB(db)
	if err != nil {
		return err
	}
	blob, _ := json.Marshal(data)
	if err := writeTar(tw, "db.json", blob); err != nil {
		return err
	}
	// DATA_DIR（证书、blobs、zones.json 等）
	if dataDir != "" {
		_ = filepath.Walk(dataDir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(filepath.Dir(dataDir), p)
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			return writeTar(tw, filepath.ToSlash(rel), b)
		})
	}
	return nil
}

// Restore 从备份包恢复：先建表（调用方应已 AutoMigrate），再逐表插入，最后解包 DATA_DIR。
func Restore(db *gorm.DB, dataDir, in string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(tr, 1<<30))
		if h.Name == "db.json" {
			var data map[string][]map[string]any
			if err := json.Unmarshal(b, &data); err != nil {
				return err
			}
			if err := importDB(db, data); err != nil {
				return err
			}
			continue
		}
		// DATA_DIR 文件
		dest := filepath.Join(filepath.Dir(dataDir), filepath.FromSlash(h.Name))
		_ = os.MkdirAll(filepath.Dir(dest), 0o755)
		_ = os.WriteFile(dest, b, 0o600)
	}
	return nil
}

func exportDB(db *gorm.DB) (map[string][]map[string]any, error) {
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return nil, err
	}
	out := map[string][]map[string]any{}
	for _, t := range tables {
		var rows []map[string]any
		if err := db.Table(t).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("导出 %s: %w", t, err)
		}
		out[t] = rows
	}
	return out, nil
}

func importDB(db *gorm.DB, data map[string][]map[string]any) error {
	for t, rows := range data {
		if len(rows) == 0 {
			continue
		}
		if err := db.Table(t).CreateInBatches(rows, 200).Error; err != nil {
			return fmt.Errorf("导入 %s: %w", t, err)
		}
	}
	return nil
}

func writeTar(tw *tar.Writer, name string, b []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(b))}); err != nil {
		return err
	}
	_, err := tw.Write(b)
	return err
}
