package silk

// C: silk/sort.c

// Insertion sort (fast for already almost sorted arrays):
// Best case:  O(n)   for an already sorted array
// Worst case: O(n^2) for an inversely sorted array
//
// Shell short:    https://en.wikipedia.org/wiki/Shell_sort

// C: void silk_insertion_sort_increasing(opus_int32 *a, opus_int *idx, const opus_int L, const opus_int K)
func InsertionSortIncreasing(
	a []int32, // I/O   Unsorted / Sorted vector
	idx []int, // O     Index vector for the sorted elements
	L int, // I     Vector length
	K int, // I     Number of correctly sorted positions
) {
	var value int32
	var i, j int

	// Safety checks
	silk_assert(K > 0)
	silk_assert(L > 0)
	silk_assert(L >= K)

	// Write start indices in index vector
	for i = 0; i < K; i++ {
		idx[i] = i
	}

	// Sort vector elements by value, increasing order
	for i = 1; i < K; i++ {
		value = a[i]
		for j = i - 1; (j >= 0) && (value < a[j]); j-- {
			a[j+1] = a[j]     // Shift value
			idx[j+1] = idx[j] // Shift index
		}
		a[j+1] = value // Write value
		idx[j+1] = i   // Write index
	}

	// If less than L values are asked for, check the remaining values,
	// but only spend CPU to ensure that the K first values are correct
	for i = K; i < L; i++ {
		value = a[i]
		if value < a[K-1] {
			for j = K - 2; (j >= 0) && (value < a[j]); j-- {
				a[j+1] = a[j]     // Shift value
				idx[j+1] = idx[j] // Shift index
			}
			a[j+1] = value // Write value
			idx[j+1] = i   // Write index
		}
	}
}

// This function is only used by the fixed-point build
//
// C: void silk_insertion_sort_decreasing_int16(opus_int16 *a, opus_int *idx, const opus_int L, const opus_int K)
func InsertionSortDecreasingInt16(
	a []int16, // I/O   Unsorted / Sorted vector
	idx []int, // O     Index vector for the sorted elements
	L int, // I     Vector length
	K int, // I     Number of correctly sorted positions
) {
	var i, j int
	var value int

	// Safety checks
	silk_assert(K > 0)
	silk_assert(L > 0)
	silk_assert(L >= K)

	// Write start indices in index vector
	for i = 0; i < K; i++ {
		idx[i] = i
	}

	// Sort vector elements by value, decreasing order
	for i = 1; i < K; i++ {
		value = int(a[i])
		for j = i - 1; (j >= 0) && (value > int(a[j])); j-- {
			a[j+1] = a[j]     // Shift value
			idx[j+1] = idx[j] // Shift index
		}
		a[j+1] = int16(value) // Write value
		idx[j+1] = i          // Write index
	}

	// If less than L values are asked for, check the remaining values,
	// but only spend CPU to ensure that the K first values are correct
	for i = K; i < L; i++ {
		value = int(a[i])
		if value > int(a[K-1]) {
			for j = K - 2; (j >= 0) && (value > int(a[j])); j-- {
				a[j+1] = a[j]     // Shift value
				idx[j+1] = idx[j] // Shift index
			}
			a[j+1] = int16(value) // Write value
			idx[j+1] = i          // Write index
		}
	}
}

// C: void silk_insertion_sort_increasing_all_values_int16(opus_int16 *a, const opus_int L)
func InsertionSortIncreasingAllValuesInt16(
	a []int16, // I/O   Unsorted / Sorted vector
	L int, // I     Vector length
) {
	var value int
	var i, j int

	// Safety checks
	silk_assert(L > 0)

	// Sort vector elements by value, increasing order
	for i = 1; i < L; i++ {
		value = int(a[i])
		for j = i - 1; (j >= 0) && (value < int(a[j])); j-- {
			a[j+1] = a[j] // Shift value
		}
		a[j+1] = int16(value) // Write value
	}
}
