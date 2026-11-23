package repository

import (
	"context"
	"errors"
	"propertyfinder/internal/domain"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ctx      = context.Background()
	errBoom  = errors.New("boom")
	tagZero  = pgconn.NewCommandTag("UPDATE 0")
	tagOne   = pgconn.NewCommandTag("UPDATE 1")
	someID   = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	otherID  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	someTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
)

func isNotFound(err error) bool {
	var notFound *domain.NotFoundError
	return errors.As(err, &notFound)
}

// TestRepositories_SingleRowLookups_MapErrNoRowsToNotFound: services and
// handlers rely on *NotFoundError (→ 404) from every by-ID lookup, and on
// any other DB error staying a plain, wrapped error (→ 500).
func TestRepositories_SingleRowLookups_MapErrNoRowsToNotFound(t *testing.T) {
	lookups := map[string]func(*fakeQuerier) error{
		"profile":          func(q *fakeQuerier) error { _, err := NewProfileRepo(q).GetByID(ctx, someID); return err },
		"profile archive":  func(q *fakeQuerier) error { _, err := NewProfileRepo(q).Archive(ctx, someID); return err },
		"criterion":        func(q *fakeQuerier) error { _, err := NewCriterionRepo(q).GetByID(ctx, someID); return err },
		"listing":          func(q *fakeQuerier) error { _, err := NewListingRepo(q).GetByID(ctx, someID); return err },
		"note":             func(q *fakeQuerier) error { _, err := NewNoteRepo(q).GetByID(ctx, someID); return err },
		"note update":      func(q *fakeQuerier) error { _, err := NewNoteRepo(q).Update(ctx, someID, "x"); return err },
		"image":            func(q *fakeQuerier) error { _, err := NewImageRepo(q).GetByID(ctx, someID); return err },
		"task":             func(q *fakeQuerier) error { _, err := NewTaskRepo(q).GetByID(ctx, someID); return err },
		"global context":   func(q *fakeQuerier) error { _, err := NewContextRepo(q).GetGlobal(ctx); return err },
		"profile context":  func(q *fakeQuerier) error { _, err := NewContextRepo(q).GetByProfileID(ctx, someID); return err },
		"context revision": func(q *fakeQuerier) error { _, err := NewContextRepo(q).GetRevisionByID(ctx, someID); return err },
		"chat session":     func(q *fakeQuerier) error { _, err := NewChatRepo(q).GetSession(ctx, someID); return err },
		"active session":   func(q *fakeQuerier) error { _, err := NewChatRepo(q).GetActiveSession(ctx, someID); return err },
		"chat message":     func(q *fakeQuerier) error { _, err := NewChatRepo(q).GetMessage(ctx, someID); return err },
	}
	for name, lookup := range lookups {
		t.Run(name, func(t *testing.T) {
			if err := lookup(&fakeQuerier{rowErr: pgx.ErrNoRows}); !isNotFound(err) {
				t.Errorf("ErrNoRows → %v, want *domain.NotFoundError", err)
			}
			err := lookup(&fakeQuerier{rowErr: errBoom})
			if isNotFound(err) || !errors.Is(err, errBoom) {
				t.Errorf("other error → %v, want a wrapped %v that is not NotFound", err, errBoom)
			}
		})
	}
}

// TestRepositories_WriteByID_ZeroRowsAffectedIsNotFound covers every
// UPDATE/DELETE that targets one row: no row hit means 404, one row means
// success, and a DB error is wrapped and never NotFound.
func TestRepositories_WriteByID_ZeroRowsAffectedIsNotFound(t *testing.T) {
	writes := map[string]func(*fakeQuerier) error{
		"profile update": func(q *fakeQuerier) error {
			return NewProfileRepo(q).Update(ctx, &domain.SearchProfile{ID: someID})
		},
		"profile delete": func(q *fakeQuerier) error { return NewProfileRepo(q).Delete(ctx, someID) },
		"profile ranking summary": func(q *fakeQuerier) error {
			return NewProfileRepo(q).SetRankingSummary(ctx, someID, "s")
		},
		"criterion update": func(q *fakeQuerier) error {
			return NewCriterionRepo(q).Update(ctx, &domain.Criterion{ID: someID})
		},
		"criterion delete": func(q *fakeQuerier) error { return NewCriterionRepo(q).Delete(ctx, someID) },
		"criterion activate": func(q *fakeQuerier) error {
			return NewCriterionRepo(q).Activate(ctx, someID, time.Now())
		},
		"listing update": func(q *fakeQuerier) error {
			return NewListingRepo(q).Update(ctx, &domain.Listing{ID: someID})
		},
		"listing delete":      func(q *fakeQuerier) error { return NewListingRepo(q).Delete(ctx, someID) },
		"listing title image": func(q *fakeQuerier) error { return NewListingRepo(q).SetTitleImage(ctx, someID, otherID) },
		"listing kurzfazit":   func(q *fakeQuerier) error { return NewListingRepo(q).SetKurzfazit(ctx, someID, "k") },
		"note delete":         func(q *fakeQuerier) error { return NewNoteRepo(q).Delete(ctx, someID) },
		"image delete":        func(q *fakeQuerier) error { return NewImageRepo(q).Delete(ctx, someID) },
		"context content":     func(q *fakeQuerier) error { return NewContextRepo(q).UpdateContent(ctx, someID, "md") },
		"chat tool calls": func(q *fakeQuerier) error {
			return NewChatRepo(q).UpdateToolCalls(ctx, someID, []byte(`[]`))
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			if err := write(&fakeQuerier{execTag: tagZero}); !isNotFound(err) {
				t.Errorf("0 rows affected → %v, want *domain.NotFoundError", err)
			}
			if err := write(&fakeQuerier{execTag: tagOne}); err != nil {
				t.Errorf("1 row affected → %v, want nil", err)
			}
			err := write(&fakeQuerier{execErr: errBoom})
			if isNotFound(err) || !errors.Is(err, errBoom) {
				t.Errorf("exec error → %v, want a wrapped %v that is not NotFound", err, errBoom)
			}
		})
	}
}

func TestNoteRepo_ScanColumnOrderAndArgs(t *testing.T) {
	q := &fakeQuerier{rowValues: []any{someID, otherID, "hello", someTime, someTime.Add(time.Hour)}}

	note, err := NewNoteRepo(q).GetByID(ctx, someID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if note.ID != someID || note.ListingID != otherID || note.Content != "hello" || !note.CreatedAt.Equal(someTime) || !note.UpdatedAt.Equal(someTime.Add(time.Hour)) {
		t.Errorf("note = %+v", note)
	}
	if len(q.args) != 1 || q.args[0] != someID {
		t.Errorf("query args = %v, want [%v]", q.args, someID)
	}
}

func TestNoteRepo_Create_ArgsMatchColumns(t *testing.T) {
	q := &fakeQuerier{}
	note := &domain.ListingNote{ID: someID, ListingID: otherID, Content: "c", CreatedAt: someTime, UpdatedAt: someTime}

	if err := NewNoteRepo(q).Create(ctx, note); err != nil {
		t.Fatalf("Create: %v", err)
	}

	want := []any{someID, otherID, "c", someTime, someTime}
	if got := q.execArgs[0]; len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("arg %d = %v, want %v", i, got[i], want[i])
			}
		}
	}
}

func TestListQueries_ReturnRowsInOrderAndNeverNil(t *testing.T) {
	t.Run("empty result is an empty slice, not nil (JSON [] not null)", func(t *testing.T) {
		notes, err := NewNoteRepo(&fakeQuerier{}).ListByListing(ctx, someID)
		if err != nil || notes == nil || len(notes) != 0 {
			t.Errorf("got (%v, %v), want empty non-nil slice", notes, err)
		}
	})
	t.Run("rows keep their order", func(t *testing.T) {
		q := &fakeQuerier{rowsValues: [][]any{
			{someID, otherID, "first", someTime, someTime},
			{otherID, otherID, "second", someTime, someTime},
		}}

		notes, err := NewNoteRepo(q).ListByListing(ctx, otherID)

		if err != nil || len(notes) != 2 || notes[0].Content != "first" || notes[1].Content != "second" {
			t.Errorf("got (%+v, %v)", notes, err)
		}
	})
}

func TestListQueries_PropagateQueryScanAndIterationErrors(t *testing.T) {
	lists := map[string]func(*fakeQuerier) error{
		"notes":     func(q *fakeQuerier) error { _, err := NewNoteRepo(q).ListByListing(ctx, someID); return err },
		"profiles":  func(q *fakeQuerier) error { _, err := NewProfileRepo(q).List(ctx); return err },
		"criteria":  func(q *fakeQuerier) error { _, err := NewCriterionRepo(q).ListByProfile(ctx, someID); return err },
		"images":    func(q *fakeQuerier) error { _, err := NewImageRepo(q).ListByListing(ctx, someID); return err },
		"sessions":  func(q *fakeQuerier) error { _, err := NewChatRepo(q).ListSessions(ctx, someID); return err },
		"messages":  func(q *fakeQuerier) error { _, err := NewChatRepo(q).ListMessages(ctx, someID); return err },
		"revisions": func(q *fakeQuerier) error { _, err := NewContextRepo(q).ListRevisions(ctx, someID); return err },
		"grades":    func(q *fakeQuerier) error { _, err := NewGradeRepo(q).ListByListing(ctx, someID); return err },
		"listings":  func(q *fakeQuerier) error { _, err := NewListingRepo(q).ListByProfile(ctx, someID, nil); return err },
		"rankable":  func(q *fakeQuerier) error { _, err := NewListingRepo(q).ListRankable(ctx, someID); return err },
	}
	for name, list := range lists {
		t.Run(name, func(t *testing.T) {
			if err := list(&fakeQuerier{queryErr: errBoom}); !errors.Is(err, errBoom) {
				t.Errorf("query error → %v, want wrapped %v", err, errBoom)
			}
			// A row whose values don't fit the scan targets must surface as an error.
			if err := list(&fakeQuerier{rowsValues: [][]any{{"wrong", "arity"}}}); err == nil {
				t.Error("scan failure swallowed")
			}
			if err := list(&fakeQuerier{rowsErr: errBoom}); !errors.Is(err, errBoom) {
				t.Errorf("rows.Err → %v, want wrapped %v", err, errBoom)
			}
		})
	}
}

func TestProfileRepo_ExistsByName_PassesExcludeIDAndReturnsFlag(t *testing.T) {
	q := &fakeQuerier{rowValues: []any{true}}
	exclude := someID

	exists, err := NewProfileRepo(q).ExistsByName(ctx, "Portugal", &exclude)

	if err != nil || !exists {
		t.Fatalf("got (%v, %v), want (true, nil)", exists, err)
	}
	if len(q.args) != 2 || q.args[0] != "Portugal" || q.args[1] != &exclude {
		t.Errorf("args = %v, want name and the exclude id pointer", q.args)
	}

	q = &fakeQuerier{rowValues: []any{false}}
	if _, err := NewProfileRepo(q).ExistsByName(ctx, "x", nil); err != nil {
		t.Fatalf("nil exclude: %v", err)
	}
	if got, ok := q.args[1].(*uuid.UUID); !ok || got != nil {
		t.Errorf("exclude arg = %#v, want a nil *uuid.UUID (SQL NULL)", q.args[1])
	}
}

func TestCountQueries_ReturnScannedCountAndWrapErrors(t *testing.T) {
	counts := map[string]func(*fakeQuerier) (int, error){
		"criteria": func(q *fakeQuerier) (int, error) { return NewCriterionRepo(q).CountByProfile(ctx, someID) },
		"images":   func(q *fakeQuerier) (int, error) { return NewImageRepo(q).CountByListing(ctx, someID) },
	}
	for name, count := range counts {
		t.Run(name, func(t *testing.T) {
			if got, err := count(&fakeQuerier{rowValues: []any{3}}); err != nil || got != 3 {
				t.Errorf("got (%d, %v), want (3, nil)", got, err)
			}
			if _, err := count(&fakeQuerier{rowErr: errBoom}); !errors.Is(err, errBoom) {
				t.Errorf("err = %v, want wrapped %v", err, errBoom)
			}
		})
	}
}

func TestListingRepo_ListByProfile_FiltersOnNeedsReevaluationOnlyWhenGiven(t *testing.T) {
	yes := true
	q := &fakeQuerier{}

	if _, err := NewListingRepo(q).ListByProfile(ctx, someID, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q.querySQL, "needs_reevaluation = $") || len(q.args) != 1 {
		t.Errorf("unfiltered query %q args %v: want no needs_reevaluation filter", q.querySQL, q.args)
	}

	if _, err := NewListingRepo(q).ListByProfile(ctx, someID, &yes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.querySQL, "needs_reevaluation = $2") || len(q.args) != 2 || q.args[1] != true {
		t.Errorf("filtered query %q args %v: want needs_reevaluation = $2 bound to true", q.querySQL, q.args)
	}
	if !strings.HasSuffix(strings.TrimSpace(q.querySQL), "ORDER BY created_at DESC") {
		t.Errorf("query %q must stay newest-first with the filter applied", q.querySQL)
	}
}

// TestListingRepo_ColumnPlaceholderArgAndScanCountsAgree guards the 32-column
// listing table: adding a field to one of the column list, the INSERT args,
// the UPDATE args or the scan list but not the others only fails at runtime.
func TestListingRepo_ColumnPlaceholderArgAndScanCountsAgree(t *testing.T) {
	columns := len(strings.Split(strings.Trim(strings.TrimSpace(listingColumns), ","), ","))

	q := &fakeQuerier{}
	if err := NewListingRepo(q).Create(ctx, &domain.Listing{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := len(q.execArgs[0]); got != columns {
		t.Errorf("Create passes %d args for %d columns", got, columns)
	}
	if got := highestPlaceholder(q.execSQL[0]); got != columns {
		t.Errorf("Create SQL uses $1..$%d for %d columns", got, columns)
	}

	q = &fakeQuerier{execTag: tagOne}
	if err := NewListingRepo(q).Update(ctx, &domain.Listing{}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// UPDATE sets every column except profile_id and created_at.
	if got, want := len(q.execArgs[0]), columns-2; got != want {
		t.Errorf("Update passes %d args, want %d", got, want)
	}
	if got, want := highestPlaceholder(q.execSQL[0]), len(q.execArgs[0]); got != want {
		t.Errorf("Update SQL uses $1..$%d but passes %d args", got, want)
	}

	q = &fakeQuerier{rowErr: errBoom}
	_, _ = NewListingRepo(q).GetByID(ctx, someID)
	if q.lastRow.scanned != columns {
		t.Errorf("scan reads %d columns, SELECT lists %d", q.lastRow.scanned, columns)
	}
}

func highestPlaceholder(sql string) int {
	highest := 0
	for _, match := range regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(sql, -1) {
		n := 0
		for _, digit := range match[1] {
			n = n*10 + int(digit-'0')
		}
		highest = max(highest, n)
	}
	return highest
}

func TestListingRepo_SetTitleImageIfUnset_ReportsWhetherItChanged(t *testing.T) {
	changed, err := NewListingRepo(&fakeQuerier{execTag: tagOne}).SetTitleImageIfUnset(ctx, someID, otherID)
	if err != nil || !changed {
		t.Errorf("1 row → (%v, %v), want (true, nil)", changed, err)
	}
	// Zero rows is "already set", not an error — callers call it unconditionally.
	changed, err = NewListingRepo(&fakeQuerier{execTag: tagZero}).SetTitleImageIfUnset(ctx, someID, otherID)
	if err != nil || changed {
		t.Errorf("0 rows → (%v, %v), want (false, nil)", changed, err)
	}
	if _, err = NewListingRepo(&fakeQuerier{execErr: errBoom}).SetTitleImageIfUnset(ctx, someID, otherID); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want wrapped %v", err, errBoom)
	}
}

func TestListingRepo_BulkStatements_BindProfileOrListingID(t *testing.T) {
	bulk := map[string]func(*fakeQuerier) error{
		"recompute scores": func(q *fakeQuerier) error { return NewListingRepo(q).RecomputeScores(ctx, someID) },
		"flag reevaluate":  func(q *fakeQuerier) error { return NewListingRepo(q).FlagNeedsReevaluation(ctx, someID) },
		"clear reevaluate": func(q *fakeQuerier) error { return NewListingRepo(q).ClearNeedsReevaluation(ctx, someID) },
	}
	for name, run := range bulk {
		t.Run(name, func(t *testing.T) {
			q := &fakeQuerier{}
			if err := run(q); err != nil {
				t.Fatalf("run: %v", err)
			}
			if len(q.execArgs) != 1 || len(q.execArgs[0]) != 1 || q.execArgs[0][0] != someID {
				t.Errorf("exec args = %v, want exactly [%v] (an unbound id would touch every profile)", q.execArgs, someID)
			}
			if err := run(&fakeQuerier{execErr: errBoom}); !errors.Is(err, errBoom) {
				t.Errorf("err = %v, want wrapped %v", err, errBoom)
			}
		})
	}
}

func TestGradeRepo_ReplaceForListing_DeletesThenInsertsEachScore(t *testing.T) {
	q := &fakeQuerier{}
	scores := []domain.CriterionScore{
		{CriterionID: someID, Grade: domain.GradeA, Reasoning: "good"},
		{CriterionID: otherID, Grade: domain.GradeF, Reasoning: "bad"},
	}

	if err := NewGradeRepo(q).ReplaceForListing(ctx, someID, scores); err != nil {
		t.Fatalf("ReplaceForListing: %v", err)
	}

	if len(q.execSQL) != 3 || !strings.HasPrefix(q.execSQL[0], "DELETE") || !strings.Contains(q.execSQL[1], "INSERT") {
		t.Fatalf("statements = %q, want DELETE then one INSERT per score", q.execSQL)
	}
	if q.execArgs[0][0] != someID {
		t.Errorf("delete bound to %v, want the listing id", q.execArgs[0][0])
	}
	// insert args: id, listing_id, criterion_id, grade, reasoning
	second := q.execArgs[2]
	if second[1] != someID || second[2] != otherID || second[3] != domain.GradeF || second[4] != "bad" {
		t.Errorf("second insert args = %v", second)
	}
	if q.execArgs[1][0] == q.execArgs[2][0] {
		t.Error("both grade rows got the same primary key")
	}
}

func TestGradeRepo_ReplaceForListing_EmptyScoresStillClearsOldGrades(t *testing.T) {
	q := &fakeQuerier{}

	if err := NewGradeRepo(q).ReplaceForListing(ctx, someID, nil); err != nil {
		t.Fatal(err)
	}

	if len(q.execSQL) != 1 || !strings.HasPrefix(q.execSQL[0], "DELETE") {
		t.Errorf("statements = %q, want the DELETE only", q.execSQL)
	}
}

func TestGradeRepo_ReplaceForListing_StopsAtFirstFailure(t *testing.T) {
	q := &fakeQuerier{execErr: errBoom, execErrAt: 2}
	scores := []domain.CriterionScore{{CriterionID: someID, Grade: domain.GradeA}, {CriterionID: otherID, Grade: domain.GradeB}}

	err := NewGradeRepo(q).ReplaceForListing(ctx, someID, scores)

	if !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want wrapped %v", err, errBoom)
	}
	if len(q.execSQL) != 2 {
		t.Errorf("ran %d statements, want to stop after the failing first insert", len(q.execSQL))
	}
}

func TestTaskRepo_ScanColumnOrder(t *testing.T) {
	total, errMsg, finished := 5, "partial", someTime.Add(time.Minute)
	q := &fakeQuerier{rowValues: []any{
		someID, domain.TaskTypeListingsReevaluateAll, domain.TaskStatusRunning, &otherID, (*uuid.UUID)(nil),
		&total, 2, 1, &errMsg, someTime, &someTime, &finished, someTime,
	}}

	task, err := NewTaskRepo(q).GetByID(ctx, someID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if task.ID != someID || task.Status != domain.TaskStatusRunning || *task.ProfileID != otherID || task.ListingID != nil ||
		*task.Total != 5 || task.Done != 2 || task.FailedCount != 1 || *task.Error != "partial" || !task.FinishedAt.Equal(finished) {
		t.Errorf("task = %+v", task)
	}
}

func TestTaskRepo_UpdateProgressAndComplete_BindArguments(t *testing.T) {
	q := &fakeQuerier{}
	msg := "failed"

	if err := NewTaskRepo(q).UpdateProgress(ctx, someID, 4, 1); err != nil {
		t.Fatal(err)
	}
	if err := NewTaskRepo(q).Complete(ctx, someID, domain.TaskStatusFailed, &msg); err != nil {
		t.Fatal(err)
	}

	if got := q.execArgs[0]; len(got) != 3 || got[0] != someID || got[1] != 4 || got[2] != 1 {
		t.Errorf("UpdateProgress args = %v, want [id 4 1]", got)
	}
	if got := q.execArgs[1]; len(got) != 3 || got[0] != someID || got[1] != domain.TaskStatusFailed || got[2] != &msg {
		t.Errorf("Complete args = %v, want [id failed &msg]", got)
	}
}

func TestContextRepo_UpdateContent_BindsContentAndID(t *testing.T) {
	q := &fakeQuerier{execTag: tagOne}

	if err := NewContextRepo(q).UpdateContent(ctx, someID, "# new"); err != nil {
		t.Fatal(err)
	}

	if got := q.execArgs[0]; len(got) != 2 || got[0] != someID || got[1] != "# new" {
		t.Errorf("args = %v, want [id \"# new\"]", got)
	}
}

func TestChatRepo_CreateMessage_ArgsFollowColumnOrder(t *testing.T) {
	q := &fakeQuerier{}
	listingID := otherID
	message := &domain.ChatMessage{
		ID: someID, SessionID: otherID, Role: "assistant", Content: "hi", ToolName: nil,
		ListingID: &listingID, CreatedAt: someTime,
	}

	if err := NewChatRepo(q).CreateMessage(ctx, message); err != nil {
		t.Fatal(err)
	}

	args := q.execArgs[0]
	columns := len(strings.Split(chatMessageColumns, ","))
	if len(args) != columns || highestPlaceholder(q.execSQL[0]) != columns {
		t.Fatalf("%d args, $%d placeholders, %d columns", len(args), highestPlaceholder(q.execSQL[0]), columns)
	}
	if args[0] != someID || args[1] != otherID || args[2] != message.Role || args[3] != "hi" || args[8] != &listingID || args[9] != someTime {
		t.Errorf("args = %v", args)
	}
}

func TestChatRepo_ScanCounts_MatchSelectedColumns(t *testing.T) {
	q := &fakeQuerier{rowErr: errBoom}
	_, _ = NewChatRepo(q).GetMessage(ctx, someID)
	if want := len(strings.Split(chatMessageColumns, ",")); q.lastRow.scanned != want {
		t.Errorf("message scan reads %d columns, SELECT lists %d", q.lastRow.scanned, want)
	}
	_, _ = NewChatRepo(q).GetSession(ctx, someID)
	if want := len(strings.Split(chatSessionColumns, ",")); q.lastRow.scanned != want {
		t.Errorf("session scan reads %d columns, SELECT lists %d", q.lastRow.scanned, want)
	}
}

func TestImageAndCriterionRepos_ScanCountsAndInsertArgsMatchColumns(t *testing.T) {
	q := &fakeQuerier{rowErr: errBoom}
	_, _ = NewImageRepo(q).GetByID(ctx, someID)
	imageColumnCount := len(strings.Split(imageColumns, ","))
	if q.lastRow.scanned != imageColumnCount {
		t.Errorf("image scan reads %d columns, SELECT lists %d", q.lastRow.scanned, imageColumnCount)
	}
	if err := NewImageRepo(q).Create(ctx, &domain.ListingImage{}); err != nil {
		t.Fatal(err)
	}
	if len(q.execArgs[0]) != imageColumnCount {
		t.Errorf("image insert passes %d args for %d columns", len(q.execArgs[0]), imageColumnCount)
	}

	_, _ = NewCriterionRepo(q).GetByID(ctx, someID)
	if err := NewCriterionRepo(q).Create(ctx, &domain.Criterion{}); err != nil {
		t.Fatal(err)
	}
	if q.lastRow.scanned != len(q.execArgs[1]) {
		t.Errorf("criterion scan reads %d columns, insert passes %d args", q.lastRow.scanned, len(q.execArgs[1]))
	}
}

// TestCriterionRepo_ListActiveByProfile_FiltersOnStatus: evaluation relies on
// this query to leave proposed criteria out (docs/adr/0024), while the plain
// list the Criteria tab shows must not filter.
func TestCriterionRepo_ListActiveByProfile_FiltersOnStatus(t *testing.T) {
	q := &fakeQuerier{}
	if _, err := NewCriterionRepo(q).ListActiveByProfile(ctx, someID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.querySQL, "status = 'active'") {
		t.Errorf("ListActiveByProfile SQL has no status filter: %s", q.querySQL)
	}
	if _, err := NewCriterionRepo(q).ListByProfile(ctx, someID); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q.querySQL, "status =") {
		t.Errorf("ListByProfile must return proposed criteria too: %s", q.querySQL)
	}
}
