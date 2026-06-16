package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
)

func main() {
	// 默认参数
	size := 20
	mode := "all"

	// 解析命令行参数
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n":
			if i+1 < len(args) {
				n, err := strconv.Atoi(args[i+1])
				if err == nil && n > 0 {
					size = n
				}
				i++
			}
		case "-m":
			if i+1 < len(args) {
				mode = args[i+1]
				i++
			}
		case "-h", "--help":
			printHelp()
			return
		}
	}

	// 生成随机数组
	rand.Seed(time.Now().UnixNano())
	original := rand.Perm(size)
	for i := range original {
		original[i] += 1 // 1..n
	}

	fmt.Printf("🧪 排序小程序 — tianxuan\n")
	fmt.Printf("═══════════════════════════════════════\n")
	fmt.Printf("📊 原始数组（n=%d）:\n%v\n\n", size, original)

	switch mode {
	case "bubble":
		runSingle("冒泡排序", bubbleSort, copySlice(original))
	case "insertion":
		runSingle("插入排序", insertionSort, copySlice(original))
	case "quick":
		runSingle("快速排序", quickSortWrap, copySlice(original))
	case "merge":
		runSingle("归并排序", mergeSortWrap, copySlice(original))
	case "all":
		all := []struct {
			name string
			fn   func([]int) []int
		}{
			{"冒泡排序", bubbleSort},
			{"插入排序", insertionSort},
			{"快速排序", quickSortWrap},
			{"归并排序", mergeSortWrap},
		}
		for _, s := range all {
			runSingle(s.name, s.fn, copySlice(original))
		}
	default:
		fmt.Fprintf(os.Stderr, "❌ 未知模式: %s\n", mode)
		os.Exit(1)
	}
}

// ---------- 排序算法 ----------

// 冒泡排序 O(n²)
func bubbleSort(arr []int) []int {
	n := len(arr)
	for i := 0; i < n-1; i++ {
		swapped := false
		for j := 0; j < n-1-i; j++ {
			if arr[j] > arr[j+1] {
				arr[j], arr[j+1] = arr[j+1], arr[j]
				swapped = true
			}
		}
		if !swapped {
			break // 提前结束
		}
	}
	return arr
}

// 插入排序 O(n²)
func insertionSort(arr []int) []int {
	for i := 1; i < len(arr); i++ {
		key := arr[i]
		j := i - 1
		for j >= 0 && arr[j] > key {
			arr[j+1] = arr[j]
			j--
		}
		arr[j+1] = key
	}
	return arr
}

// 快速排序 O(n log n)
func quickSortWrap(arr []int) []int {
	quickSort(arr, 0, len(arr)-1)
	return arr
}

func quickSort(arr []int, low, high int) {
	if low < high {
		p := partition(arr, low, high)
		quickSort(arr, low, p-1)
		quickSort(arr, p+1, high)
	}
}

func partition(arr []int, low, high int) int {
	pivot := arr[high]
	i := low - 1
	for j := low; j < high; j++ {
		if arr[j] <= pivot {
			i++
			arr[i], arr[j] = arr[j], arr[i]
		}
	}
	arr[i+1], arr[high] = arr[high], arr[i+1]
	return i + 1
}

// 归并排序 O(n log n)
func mergeSortWrap(arr []int) []int {
	return mergeSort(arr)
}

func mergeSort(arr []int) []int {
	if len(arr) <= 1 {
		return arr
	}
	mid := len(arr) / 2
	left := mergeSort(copySlice(arr[:mid]))
	right := mergeSort(copySlice(arr[mid:]))
	return merge(left, right)
}

func merge(left, right []int) []int {
	result := make([]int, 0, len(left)+len(right))
	i, j := 0, 0
	for i < len(left) && j < len(right) {
		if left[i] <= right[j] {
			result = append(result, left[i])
			i++
		} else {
			result = append(result, right[j])
			j++
		}
	}
	result = append(result, left[i:]...)
	result = append(result, right[j:]...)
	return result
}

// ---------- 辅助函数 ----------

func copySlice(src []int) []int {
	dst := make([]int, len(src))
	copy(dst, src)
	return dst
}

func runSingle(name string, sortFn func([]int) []int, data []int) {
	fmt.Printf("▸ %s\n", name)
	fmt.Printf("  ────────────────────────────────\n")

	start := time.Now()
	result := sortFn(data)
	elapsed := time.Since(start)

	fmt.Printf("  结果: %v\n", result)
	fmt.Printf("  耗时: %v\n", elapsed)
	fmt.Printf("  有序: %v\n\n", isSorted(result))
}

func isSorted(arr []int) bool {
	for i := 1; i < len(arr); i++ {
		if arr[i-1] > arr[i] {
			return false
		}
	}
	return true
}

func printHelp() {
	fmt.Println("排序小程序 - 使用说明")
	fmt.Println()
	fmt.Println("用法: go run main.go [选项]")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  -n <数量>    数组大小（默认 20）")
	fmt.Println("  -m <模式>    排序模式：bubble | insertion | quick | merge | all（默认 all）")
	fmt.Println("  -h, --help   显示此帮助")
}
