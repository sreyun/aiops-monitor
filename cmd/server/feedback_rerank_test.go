package main

import "testing"

// TestFeedbackAdjustedDistance 验证反馈→有效距离的折算：👍 减距（上浮）、👎 加距（下沉）、
// 未验证反馈轻微下沉、未知反馈按中性处理。有效距离只用于排序，不改动展示用的原始距离。
func TestFeedbackAdjustedDistance(t *testing.T) {
	const eps = 1e-9
	cases := []struct {
		raw      float64
		feedback string
		want     float64
	}{
		{0.30, "helpful", 0.30 - feedbackHelpfulBonus},
		{0.30, "unhelpful", 0.30 + feedbackUnhelpfulPenalty},
		{0.30, "", 0.30 + feedbackPendingPenalty},
		{0.30, "pending", 0.30 + feedbackPendingPenalty},
		{0.30, "garbage", 0.30}, // 未知反馈按中性处理
	}
	for _, c := range cases {
		got := feedbackAdjustedDistance(c.raw, c.feedback)
		if diff := got - c.want; diff > eps || diff < -eps {
			t.Errorf("feedbackAdjustedDistance(%v,%q)=%v want %v", c.raw, c.feedback, got, c.want)
		}
	}
}

// TestRerankByFeedback 验证反馈驱动的检索重排闭环：👍 案例上浮、👎 案例下沉并可被挤出 Top-N，
// 无反馈时保持纯距离升序（回归保护），且原始 Distance 永不被修改。
func TestRerankByFeedback(t *testing.T) {
	// 场景1：👎 案例即便原始距离最近，也被惩罚挤到最后（低于待验证案例）。
	t.Run("unhelpful_demoted", func(t *testing.T) {
		in := []similarCase{
			{ID: 1, Distance: 0.10, Feedback: "unhelpful"}, // 有效 0.30
			{ID: 2, Distance: 0.20, Feedback: ""},          // 有效 0.24
			{ID: 3, Distance: 0.25, Feedback: ""},          // 有效 0.29
		}
		assertOrder(t, rerankByFeedback(in, 3), []int64{2, 3, 1})
	})

	// 场景2：👎 案例被挤出 Top-N（limit=2 → 只剩两个待验证案例）。
	t.Run("unhelpful_filtered_out_of_topN", func(t *testing.T) {
		in := []similarCase{
			{ID: 1, Distance: 0.10, Feedback: "unhelpful"}, // 有效 0.30
			{ID: 2, Distance: 0.20, Feedback: ""},          // 有效 0.24
			{ID: 3, Distance: 0.25, Feedback: ""},          // 有效 0.29
		}
		got := rerankByFeedback(in, 2)
		if len(got) != 2 {
			t.Fatalf("want 2 results, got %d", len(got))
		}
		for _, c := range got {
			if c.ID == 1 {
				t.Errorf("被标 👎 的案例不应出现在 Top-2: %+v", got)
			}
		}
	})

	// 场景3：👍 案例上浮（原始距离略远，仍排到中性案例之前）。
	t.Run("helpful_boosted", func(t *testing.T) {
		in := []similarCase{
			{ID: 1, Distance: 0.28, Feedback: ""},        // 有效 0.28
			{ID: 2, Distance: 0.30, Feedback: "helpful"}, // 有效 0.25
		}
		assertOrder(t, rerankByFeedback(in, 2), []int64{2, 1})
	})

	// 场景4：无任何反馈 → 保持纯距离升序（行为不回归）。
	t.Run("no_feedback_preserves_distance_order", func(t *testing.T) {
		in := []similarCase{
			{ID: 1, Distance: 0.10, Feedback: ""},
			{ID: 2, Distance: 0.20, Feedback: ""},
			{ID: 3, Distance: 0.05, Feedback: ""},
		}
		assertOrder(t, rerankByFeedback(in, 3), []int64{3, 1, 2})
	})

	// 场景5：limit 截断。
	t.Run("limit_truncates", func(t *testing.T) {
		in := []similarCase{
			{ID: 1, Distance: 0.10}, {ID: 2, Distance: 0.20},
			{ID: 3, Distance: 0.30}, {ID: 4, Distance: 0.40},
		}
		if got := rerankByFeedback(in, 2); len(got) != 2 {
			t.Fatalf("limit=2 应返回 2 条, 得到 %d", len(got))
		}
	})

	// 场景6：原始 Distance 不被反馈修改（展示给用户的相似度必须真实）。
	t.Run("raw_distance_untouched", func(t *testing.T) {
		in := []similarCase{{ID: 1, Distance: 0.30, Feedback: "helpful"}}
		got := rerankByFeedback(in, 1)
		if got[0].Distance != 0.30 {
			t.Errorf("原始 Distance 应保持 0.30（仅排序用有效距离），得到 %v", got[0].Distance)
		}
	})
}

func assertOrder(t *testing.T, got []similarCase, wantIDs []int64) {
	t.Helper()
	if len(got) != len(wantIDs) {
		t.Fatalf("结果条数 %d != 期望 %d: %+v", len(got), len(wantIDs), got)
	}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Fatalf("排序错误：位置 %d 应为 ID=%d，实际 %d（完整=%+v）", i, id, got[i].ID, got)
		}
	}
}
