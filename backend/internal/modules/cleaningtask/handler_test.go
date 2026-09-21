package cleaningtask_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/modules/cleaningtask"
	"github.com/drainage/desilting/internal/testsupport"
)

func newTaskHandlerApp(fixture *testsupport.Fixture) *fiber.App {
	// 与生产装配一致：业务错误（如筛选条件冲突）经统一信封写出真实 HTTP 状态码。
	app := fiber.New(fiber.Config{ErrorHandler: httpx.WriteError})
	cleaningtask.Register(app.Group("/api/v1"), fixture.DB, fixture.Segments)
	return app
}

func getStatus(t *testing.T, app *fiber.App, target string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	resp, err := app.Test(req, -1)
	testsupport.RequireNoError(t, err)
	return resp.StatusCode
}

func TestTaskListRejectsConflictingPlanRange(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	app := newTaskHandlerApp(fixture)

	// 互相冲突的条件（计划开始日期起晚于止）必须直接以 400 说明。
	if code := getStatus(t, app, "/api/v1/cleaning-tasks?planFrom=2026-09-10&planTo=2026-09-01"); code != fiber.StatusBadRequest {
		t.Fatalf("冲突的计划时间段应返回 400，实际 %d", code)
	}
}

func TestTaskListRejectsBadDateFormat(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	app := newTaskHandlerApp(fixture)

	if code := getStatus(t, app, "/api/v1/cleaning-tasks?planFrom=2026-9-1"); code != fiber.StatusBadRequest {
		t.Fatalf("错误日期格式应返回 400，实际 %d", code)
	}
}
