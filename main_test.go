package main

import "testing"

// TestTwoSum 覆盖核心逻辑与边界场景，确保正确性与稳定性
func TestTwoSum(t *testing.T) {
	nums := []int{2, 7, 11, 15}
	i, j, ok := twoSum(nums, 9)
	if !ok || !(i == 0 && j == 1) {
		t.Fatalf("expect indices (0,1), got (%d,%d), ok=%v", i, j, ok)
	}

	i, j, ok = twoSum(nums, 26)
	if !ok || !(i == 2 && j == 3) {
		t.Fatalf("expect indices (2,3), got (%d,%d), ok=%v", i, j, ok)
	}

	// 不存在解的情况
	i, j, ok = twoSum(nums, 100)
	if ok || !(i == -1 && j == -1) {
		t.Fatalf("expect no solution, got (%d,%d), ok=%v", i, j, ok)
	}

	// 重复值与自身不重复使用的场景
	nums = []int{3, 3}
	i, j, ok = twoSum(nums, 6)
	if !ok || !(i == 0 && j == 1) {
		t.Fatalf("expect indices (0,1), got (%d,%d), ok=%v", i, j, ok)
	}
}
