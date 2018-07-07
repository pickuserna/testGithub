package main

import (
	"fmt"
	"time"
)

func assertEqual(a interface{}, b interface{}) {
	if a != b {
		panic(fmt.Sprint("Expected ", a, ", but got ", b))
	}
}

func fib(n int) int {
	if n < 2 {
		return 1
	}
	return fib(n - 1) + fib(n - 2)
}

func addOne(x int) int {
	return x + 1
}

func testMath() {
	assertEqual(2, 1 + 1)
}

func testFunctions() {
	assertEqual(5, fib(4))
	assertEqual(2, addOne(1))
}

func testVariables() {
	var x, y int
	assertEqual(0, x)
	assertEqual(0, y)
}

func testForLoop() {
	result := 1
	var i int
	for {
		result *= 2
		i++
		if i >= 5 {
			break
		}
	}
	assertEqual(32, result)

	sum := 0
	for j := 0; j <= 5; j++ {
		sum += j
	}
	assertEqual(15, sum)
}

func testSlices() {
	nums := []int{4, 8, 15, 16, 23, 42}
	assertEqual(15, nums[2])
	nums[3] = 5
	assertEqual(5, nums[3])
}

type SampleStruct struct {
	x int
}

func testStruct() {
	sample := SampleStruct{}
	assertEqual(0, sample.x)
	sample.x = 3
	assertEqual(3, sample.x)
	var sample2 SampleStruct
	assertEqual(0, sample2.x)
	sample3 := SampleStruct{
		7,
	}
	assertEqual(7, sample3.x)
	sample4 := SampleStruct{
		x: 12,
	}
	assertEqual(12, sample4.x)
}

type StructWithMethods struct {
	val int
}

func (s *StructWithMethods) getValPlusOne() int {
	return s.val + 1
}

// This method passes the receiver by value, so we end up modifying the copy,
// which doesn't do anything.
func (s StructWithMethods) incorrectlySetValToOne() {
	s.val = 1
}

func (s *StructWithMethods) correctlySetValToOne() {
	s.val = 1
}

func (s StructWithMethods) getVal() int {
	return s.val
}

func testMethods() {
	sample := StructWithMethods{3}
	getFn := sample.getVal
	assertEqual(4, sample.getValPlusOne())
	sample.incorrectlySetValToOne()
	assertEqual(3, sample.val)
	sample.correctlySetValToOne()
	assertEqual(1, sample.val)
	// The pass-by-value happens eagerly when getFn is created, not when it
	// is called.
	assertEqual(3, getFn())
}

func main() {
	start := time.Now()
	testMath()
	testFunctions()
	testVariables()
	testForLoop()
	testSlices()
	testStruct()
	testMethods()
	fmt.Println("Pass!")
	fmt.Println("Took ", time.Since(start))
}