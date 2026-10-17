package app

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"

	"example.com/jana-locate-cli/internal/api"
	"modernc.org/sqlite"
)

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("jana_fold", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if text, ok := args[0].(string); ok {
			return strings.ToLower(text), nil
		}
		return "", nil
	})
}

type BrowseOptions struct {
	Offset, Limit int
	Query         string
	ReviewOnly    bool
	Sort          string
}

// Browse searches the local snapshot, preserving original positions and exact source IDs.
// Search is Unicode case-insensitive and treats SQL wildcard characters literally.
func (s *Store) Browse(ctx context.Context, id string, o BrowseOptions) ([]api.Row, int, error) {
	if o.Limit < 1 || o.Limit > maxRecords || o.Offset < 0 {
		return nil, 0, errors.New("Некорректные параметры страницы")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	if _, err = readRun(tx, ctx, id); err != nil {
		return nil, 0, err
	}
	where := "run_id=?"
	args := []any{id}
	if o.ReviewOnly {
		where += " AND reviewed=0 AND (result IS NULL OR json_extract(result,'$.status')<>'matched')"
	}
	if q := strings.ToLower(strings.TrimSpace(o.Query)); q != "" {
		where += " AND instr(jana_fold(coalesce(raw_address,'')||' '||coalesce(json_extract(result,'$.province'),'')||' '||coalesce(json_extract(result,'$.city'),'')),?)>0"
		args = append(args, q)
	}
	order := "position"
	switch o.Sort {
	case "status":
		order = "reviewed,CASE json_extract(result,'$.status') WHEN 'matched' THEN 1 ELSE 0 END,position"
	case "region":
		order = "jana_fold(coalesce(json_extract(result,'$.province'),'')),jana_fold(coalesce(json_extract(result,'$.city'),'')),position"
	case "risk":
		order = "reviewed,CASE json_extract(result,'$.status') WHEN 'matched' THEN 1 ELSE 0 END,coalesce(json_extract(result,'$.confidence'),0),position"
	}
	var total int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM records WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, o.Limit, o.Offset)
	rows, err := tx.QueryContext(ctx, "SELECT position,source_id,raw_address,result,reviewed,revision,input_id,city_selection FROM records WHERE "+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, 0, err
	}
	out, err := scanRows(rows)
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
