package silk

// C: silk/fixed/solve_LS_FIX.c

// *****************************
// Internal function headers
// *****************************

// C: typedef struct { opus_int32 Q36_part; opus_int32 Q48_part; } inv_D_t;
type inv_D_t struct {
	Q36_part int32
	Q48_part int32
}

// Solves Ax = b, assuming A is symmetric
//
// C: void silk_solve_LDL_FIX(opus_int32 *A, opus_int M, const opus_int32 *b, opus_int32 *x_Q16)
func SolveLDLFIX(
	A []int32, // I    Pointer to symetric square matrix A
	M int, // I    Size of matrix
	b []int32, // I    Pointer to b vector
	x_Q16 []int32, // O    Pointer to x solution vector
) {
	var L_Q16 []int32
	var Y [MAX_MATRIX_SIZE]int32
	var inv_D [MAX_MATRIX_SIZE]inv_D_t

	silk_assert(M <= MAX_MATRIX_SIZE)
	L_Q16 = make([]int32, M*M)

	// **************************************************
	// Factorize A by LDL such that A = L*D*L',
	// where L is lower triangular with ones on diagonal
	// ***************************************************
	ldlFactorizeFIX(A, M, L_Q16, inv_D[:])

	// ***************************************************
	// * substitute D*L'*x = Y. ie:
	// L*D*L'*x = b => L*Y = b <=> Y = inv(L)*b
	// *****************************************************
	lsSolveFirstFIX(L_Q16, M, b, Y[:])

	// ***************************************************
	// D*L'*x = Y <=> L'*x = inv(D)*Y, because D is
	// diagonal just multiply with 1/d_i
	// ***************************************************
	lsDivideQ16FIX(Y[:], inv_D[:], M)

	// ***************************************************
	// x = inv(L') * inv(D) * Y
	// ****************************************************
	lsSolveLastFIX(L_Q16, M, Y[:], x_Q16)
}

// Factorize square matrix A into LDL form
//
// C: static OPUS_INLINE void silk_LDL_factorize_FIX(opus_int32 *A, opus_int M, opus_int32 *L_Q16, inv_D_t *inv_D)
func ldlFactorizeFIX(
	A []int32, // I/O Pointer to Symetric Square Matrix
	M int, // I   Size of Matrix
	L_Q16 []int32, // I/O Pointer to Square Upper triangular Matrix
	inv_D []inv_D_t, // I/O Pointer to vector holding inverted diagonal elements of D
) {
	var i, j, k, status, loop_count int
	var ptr1, ptr2 []int32
	var diag_min_value, tmp_32, err int32
	var v_Q0, D_Q0 [MAX_MATRIX_SIZE]int32
	var one_div_diag_Q36, one_div_diag_Q40, one_div_diag_Q48 int32

	silk_assert(M <= MAX_MATRIX_SIZE)

	status = 1
	diag_min_value = max_32(SMMUL(ADD_SAT32(A[0], A[SMULBB(M, M)-1]), SILK_FIX_CONST(FIND_LTP_COND_FAC, 31)), 1<<9)
	for loop_count = 0; loop_count < M && status == 1; loop_count++ {
		status = 0
		for j = 0; j < M; j++ {
			ptr1 = matrix_adr(L_Q16, j, 0, M)
			tmp_32 = 0
			for i = 0; i < j; i++ {
				v_Q0[i] = SMULWW(D_Q0[i], ptr1[i])        // Q0
				tmp_32 = SMLAWW(tmp_32, v_Q0[i], ptr1[i]) // Q0
			}
			tmp_32 = SUB32(*matrix_ptr(A, j, j, M), tmp_32)

			if tmp_32 < diag_min_value {
				tmp_32 = SUB32(SMULBB(loop_count+1, diag_min_value), tmp_32)
				// Matrix not positive semi-definite, or ill conditioned
				for i = 0; i < M; i++ {
					*matrix_ptr(A, i, i, M) = ADD32(*matrix_ptr(A, i, i, M), tmp_32)
				}
				status = 1
				break
			}
			D_Q0[j] = tmp_32 // always < max(Correlation)

			// two-step division
			one_div_diag_Q36 = INVERSE32_varQ(tmp_32, 36)               // Q36
			one_div_diag_Q40 = LSHIFT(one_div_diag_Q36, 4)              // Q40
			err = SUB32(int32(1)<<24, SMULWW(tmp_32, one_div_diag_Q40)) // Q24
			one_div_diag_Q48 = SMULWW(err, one_div_diag_Q40)            // Q48

			// Save 1/Ds
			inv_D[j].Q36_part = one_div_diag_Q36
			inv_D[j].Q48_part = one_div_diag_Q48

			*matrix_ptr(L_Q16, j, j, M) = 65536 // 1.0 in Q16
			ptr1 = matrix_adr(A, j, 0, M)
			ptr2 = matrix_adr(L_Q16, j+1, 0, M)
			for i = j + 1; i < M; i++ {
				tmp_32 = 0
				for k = 0; k < j; k++ {
					tmp_32 = SMLAWW(tmp_32, v_Q0[k], ptr2[k]) // Q0
				}
				tmp_32 = SUB32(ptr1[i], tmp_32) // always < max(Correlation)

				// tmp_32 / D_Q0[j] : Divide to Q16
				*matrix_ptr(L_Q16, i, j, M) = ADD32(SMMUL(tmp_32, one_div_diag_Q48),
					RSHIFT(SMULWW(tmp_32, one_div_diag_Q36), 4))

				// go to next column
				ptr2 = ptr2[M:]
			}
		}
	}

	silk_assert(status == 0)
}

// C: static inline void silk_LS_divide_Q16_FIX(opus_int32 T[], inv_D_t *inv_D, opus_int M)
func lsDivideQ16FIX(
	T []int32, // I/O  Numenator vector
	inv_D []inv_D_t, // I    1 / D vector
	M int, // I    dimension
) {
	var i int
	var tmp_32 int32
	var one_div_diag_Q36, one_div_diag_Q48 int32

	for i = 0; i < M; i++ {
		one_div_diag_Q36 = inv_D[i].Q36_part
		one_div_diag_Q48 = inv_D[i].Q48_part

		tmp_32 = T[i]
		T[i] = ADD32(SMMUL(tmp_32, one_div_diag_Q48), RSHIFT(SMULWW(tmp_32, one_div_diag_Q36), 4))
	}
}

// Solve Lx = b, when L is lower triangular and has ones on the diagonal
//
// C: static inline void silk_LS_SolveFirst_FIX(const opus_int32 *L_Q16, opus_int M, const opus_int32 *b, opus_int32 *x_Q16)
func lsSolveFirstFIX(
	L_Q16 []int32, // I    Pointer to Lower Triangular Matrix
	M int, // I    Dim of Matrix equation
	b []int32, // I    b Vector
	x_Q16 []int32, // O    x Vector
) {
	var i, j int
	var ptr32 []int32
	var tmp_32 int32

	for i = 0; i < M; i++ {
		ptr32 = matrix_adr(L_Q16, i, 0, M)
		tmp_32 = 0
		for j = 0; j < i; j++ {
			tmp_32 = SMLAWW(tmp_32, ptr32[j], x_Q16[j])
		}
		x_Q16[i] = SUB32(b[i], tmp_32)
	}
}

// Solve L^t*x = b, where L is lower triangular with ones on the diagonal
//
// C: static inline void silk_LS_SolveLast_FIX(const opus_int32 *L_Q16, const opus_int M, const opus_int32 *b, opus_int32 *x_Q16)
func lsSolveLastFIX(
	L_Q16 []int32, // I    Pointer to Lower Triangular Matrix
	M int, // I    Dim of Matrix equation
	b []int32, // I    b Vector
	x_Q16 []int32, // O    x Vector
) {
	var i, j int
	var ptr32 []int32
	var tmp_32 int32

	for i = M - 1; i >= 0; i-- {
		ptr32 = matrix_adr(L_Q16, 0, i, M)
		tmp_32 = 0
		for j = M - 1; j > i; j-- {
			tmp_32 = SMLAWW(tmp_32, ptr32[SMULBB(j, M)], x_Q16[j])
		}
		x_Q16[i] = SUB32(b[i], tmp_32)
	}
}
