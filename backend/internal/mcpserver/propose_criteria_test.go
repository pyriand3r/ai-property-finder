package mcpserver

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"propertyfinder/internal/db/dbtest"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectMCP starts the real MCP handler over HTTP and returns a connected
// client — the path an external client such as Claude Desktop takes.
func connectMCP(t *testing.T, pool *pgxpool.Pool) *mcp.ClientSession {
	t.Helper()
	server := httptest.NewServer(NewHandler(Deps{
		Criteria:   service.NewCriterionService(pool),
		Evaluation: service.NewEvaluationService(pool, nil, nil), // grading tools never call the LLM provider
	}))
	t.Cleanup(server.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callProposeCriteria(t *testing.T, session *mcp.ClientSession, args map[string]any) (*mcp.CallToolResult, CriterionOutput) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "propose_criteria", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var out CriterionOutput
	if !result.IsError {
		raw, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode result: %v", err)
		}
	}
	return result, out
}

// TestProposeCriteria_MCP_WritesOnlyProposedCriteria is the issue #29
// contract for external clients: a criterion added over MCP comes back as
// "proposed"; refining a proposal works; touching an active criterion is a
// tool error and changes nothing.
func TestProposeCriteria_MCP_WritesOnlyProposedCriteria(t *testing.T) {
	pool := dbtest.NewPool(t)
	ctx := context.Background()
	profileID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO search_profile (id, name) VALUES ($1, 'P')`, profileID); err != nil {
		t.Fatal(err)
	}
	criteria := service.NewCriterionService(pool)
	active, err := criteria.Create(ctx, profileID, "Preis", "alt", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	session := connectMCP(t, pool)

	// add -> proposed
	result, added := callProposeCriteria(t, session, map[string]any{
		"profile_id": profileID.String(), "action": "add", "name": "Lage", "description": "ruhig", "weight": 4,
	})
	if result.IsError || added.Status != string(domain.CriterionStatusProposed) || added.Weight != 4 {
		t.Fatalf("add: isError=%v output=%+v, want a proposed criterion with weight 4", result.IsError, added)
	}

	// edit the proposal -> still proposed
	_, edited := callProposeCriteria(t, session, map[string]any{
		"profile_id": profileID.String(), "action": "edit", "criterion_id": added.ID, "name": "Lage", "description": "sehr ruhig", "weight": 5,
	})
	if edited.Description != "sehr ruhig" || edited.Status != string(domain.CriterionStatusProposed) {
		t.Errorf("edit of a proposal: %+v, want new description and status proposed", edited)
	}

	// edit an active criterion -> error, nothing changed
	result, _ = callProposeCriteria(t, session, map[string]any{
		"profile_id": profileID.String(), "action": "edit", "criterion_id": active.ID.String(), "name": "Preis", "description": "gekapert", "weight": 1,
	})
	if !result.IsError {
		t.Error("edit of an active criterion succeeded, want a tool error")
	}
	all, _ := criteria.List(ctx, profileID)
	for _, criterion := range all {
		if criterion.ID == active.ID && (criterion.Description != "alt" || criterion.Weight != 3) {
			t.Errorf("active criterion was changed over MCP: %+v", criterion)
		}
	}

	// duplicate name (case-insensitive, vs active and vs proposed) -> error
	for _, name := range []string{"preis", "LAGE"} {
		result, _ = callProposeCriteria(t, session, map[string]any{"profile_id": profileID.String(), "action": "add", "name": name})
		if !result.IsError {
			t.Errorf("duplicate name %q accepted, want a tool error", name)
		}
	}
	if all, _ = criteria.List(ctx, profileID); len(all) != 2 {
		t.Errorf("profile has %d criteria, want 2 (Preis + Lage)", len(all))
	}
}
