// Code generator
// Convention:
//  We SHOULD use the word "emit" for the meaning of "output assembly code",
//  NOT for "load something to %rax".
//  Such usage would make much confusion.

package main

import (
	"fmt"
	"os"
)

const IntSize int = 8 // 64-bit (8 bytes)
const ptrSize int = 8
const sliceWidth int = 3
const interfaceWidth int = 3
const mapWidth int = 3
const sliceSize int = IntSize + ptrSize + ptrSize

func emitNewline() {
	var b []byte = []byte{'\n'}
	os.Stdout.Write(b)
}

func emitOut(format string, v ...interface{}) {
	s := fmt.Sprintf(format, v...)
	var b []byte = []byte(s)
	os.Stdout.Write(b)
}

var gasIndentLevel int = 1

func emit(format string, v ...interface{}) {
	var format2 string = format

	for i := 0; i < gasIndentLevel; i++ {
		format2 = "  " + format2
	}

	frmt := format2+"\n"
	emitOut(frmt, v...)
}

func emitWithoutIndent(format string, v ...interface{}) {
	frmt := format+"\n"
	emitOut(frmt, v...)
}

// Mytype.method -> Mytype#method
func getMethodUniqueName(gtype *Gtype, fname identifier) string {
	assertNotNil(gtype != nil, nil)
	var typename identifier
	if gtype.kind == G_POINTER {
		typename = gtype.origType.relation.name
	} else {
		typename = gtype.relation.name
	}
	return string(typename) + "$" + string(fname)
}

// "main","f1" -> "main.f1"
func getFuncSymbol(pkg identifier, fname string) string {
	if pkg == "libc" {
		return fname
	}
	if pkg == "" {
		pkg = ""
	}
	return fmt.Sprintf("%s.%s", pkg, fname)
}

func (f *DeclFunc) getSymbol() string {
	if f.receiver != nil {
		// method
		return getFuncSymbol(f.pkg, getMethodUniqueName(f.receiver.gtype, f.fname))
	}

	// other functions
	return getFuncSymbol(f.pkg, string(f.fname))
}

func align(n int, m int) int {
	remainder := n % m
	if remainder == 0 {
		return n
	} else {
		return n - remainder + m
	}
}

func emitFuncEpilogue(labelDeferHandler string, stmtDefer *StmtDefer) {
	emitNewline()
	emit("# func epilogue")
	// every function has a defer handler
	emit("%s: # defer handler", labelDeferHandler)

	// if the function has a defer statement, jump to there
	if stmtDefer != nil {
		emit("jmp %s", stmtDefer.label)
	}

	emit("LEAVE_AND_RET")
}

func (structfield *ExprStructField) calcOffset() {
	fieldType := structfield.getGtype()
	if fieldType.offset != undefinedSize {
		return
	}

	structType := structfield.strct.getGtype()
	switch structType.getKind() {
	case G_POINTER:
		origType := structType.origType.relation.gtype
		if origType.size == undefinedSize {
			origType.calcStructOffset()
		}
	case G_STRUCT:
		structType.calcStructOffset()
	default:
		errorf("invalid case")
	}

	if fieldType.offset == undefinedSize {
		errorf("filed type %s [named %s] offset should not be minus.", fieldType.String(), structfield.fieldname)
	}
}

func emit_intcast(gtype *Gtype) {
	if gtype.getKind() == G_BYTE {
		emit("CAST_BYTE_TO_INT")
	}
}

func emit_comp_primitive(inst string, binop *ExprBinop) {
	emit("# emit_comp_primitive")
	binop.left.emit()
	if binop.left.getGtype().getKind() == G_BYTE {
		emit_intcast(binop.left.getGtype())
	}
	emit("PUSH_8 # left") // left
	binop.right.emit()
	if binop.right.getGtype().getKind() == G_BYTE {
		emit_intcast(binop.right.getGtype())
	}
	emit("PUSH_8 # right") // right
	emit("CMP_FROM_STACK %s", inst)
}

var labelSeq = 0

func makeLabel() string {
	r := fmt.Sprintf(".L%d", labelSeq)
	labelSeq++
	return r
}

func (ast *StmtInc) emit() {
	emitIncrDecl("ADD_NUMBER 1", ast.operand)
}
func (ast *StmtDec) emit() {
	emitIncrDecl("SUB_NUMBER 1", ast.operand)
}

// https://golang.org/ref/spec#IncDecStmt
// As with an assignment, the operand must be addressable or a map index expression.
func emitIncrDecl(inst string, operand Expr) {
	operand.emit()
	emit(inst)

	left := operand
	emitSave(left)
}

// e.g. *x = 1, or *x++
func (uop *ExprUop) emitSave() {
	emit("# *ExprUop.emitSave()")
	assert(uop.op == "*", uop.tok, "uop op should be *")
	emit("PUSH_8")
	uop.operand.emit()
	emit("PUSH_8")
	emit("STORE_8_INDIRECT_FROM_STACK")
}

// e.g. x = 1
func (rel *Relation) emitSave() {
	assert(rel.expr != nil, rel.token(), "left.rel.expr is nil")
	variable := rel.expr.(*ExprVariable)
	variable.emitOffsetSave(variable.getGtype().getSize(), 0, false)
}

func (variable *ExprVariable) emitOffsetSave(size int, offset int, forceIndirection bool) {
	emit("# ExprVariable.emitOffsetSave(size %d, offset %d)", size, offset)
	assert(0 <= size && size <= 8, variable.token(), fmt.Sprintf("invalid size %d", size))
	if variable.getGtype().kind == G_POINTER && (offset > 0 || forceIndirection) {
		assert(variable.getGtype().kind == G_POINTER, variable.token(), "")
		emit("PUSH_8")
		variable.emit()
		emit("ADD_NUMBER %d", offset)
		emit("PUSH_8")
		emit("STORE_8_INDIRECT_FROM_STACK")
		return
	}
	if variable.isGlobal {
		emit("STORE_%d_TO_GLOBAL %s %d", size, variable.varname, offset)
	} else {
		emit("STORE_%d_TO_LOCAL %d+%d", size, variable.offset, offset)
	}
}

func (binop *ExprBinop) emitCompareStrings() {
	emit("# emitCompareStrings")
	var equal bool
	switch binop.op {
	case "<":
		TBI(binop.token(), "")
	case ">":
		TBI(binop.token(), "")
	case "<=":
		TBI(binop.token(), "")
	case ">=":
		TBI(binop.token(), "")
	case "!=":
		equal = false
	case "==":
		equal = true
	}

	labelElse := makeLabel()
	labelEnd := makeLabel()

	binop.left.emit()

	// convert nil to the empty string
	emit("CMP_EQ_ZERO")
	emit("TEST_IT")
	emit("LOAD_NUMBER 0")
	emit("je %s", labelElse)
	emitEmptyString()
	emit("jmp %s", labelEnd)
	emit("%s:", labelElse)
	binop.left.emit()
	emit("%s:", labelEnd)
	emit("PUSH_8")

	binop.right.emit()
	emit("PUSH_8")
	emitStringsEqualFromStack(equal)
}

func emitConvertNilToEmptyString() {
	emit("# emitConvertNilToEmptyString")

	emit("PUSH_8")
	emit("# convert nil to an empty string")
	emit("TEST_IT")
	emit("pop %%rax")
	labelEnd := makeLabel()
	emit("jne %s # jump if not nil", labelEnd)
	emit("# if nil then")
	emitEmptyString()
	emit("%s:", labelEnd)
}

// call strcmp
func emitStringsEqualFromStack(equal bool) {
	emit("pop %%rax") // left

	emitConvertNilToEmptyString()
	emit("mov %%rax, %%rcx")
	emit("pop %%rax # right string")
	emit("push %%rcx")
	emitConvertNilToEmptyString()

	emit("PUSH_8")
	emit("POP_TO_ARG_0")
	emit("POP_TO_ARG_1")
	emit("FUNCALL strcmp")
	if equal {
		emit("CMP_EQ_ZERO") // retval == 0
	} else {
		emit("CMP_NE_ZERO") // retval != 0
	}
}

func (binop *ExprBinop) emitComp() {
	emit("# emitComp")
	if binop.left.getGtype().isString() {
		binop.emitCompareStrings()
		return
	}

	var instruction string
	switch binop.op {
	case "<":
		instruction = "setl"
	case ">":
		instruction = "setg"
	case "<=":
		instruction = "setle"
	case ">=":
		instruction = "setge"
	case "!=":
		instruction = "setne"
	case "==":
		instruction = "sete"
	}

	emit_comp_primitive(instruction, binop)
}

func emitStringConcate(left Expr, right Expr) {
	emit("# emitStringConcate")
	left.emit()
	emit("PUSH_8 # left string")

	emit("PUSH_8")
	emit("POP_TO_ARG_0")
	emit("FUNCALL strlen # get left len")

	emit("PUSH_8 # left len")
	right.emit()
	emit("PUSH_8 # right string")
	emit("PUSH_8")
	emit("POP_TO_ARG_0")
	emit("FUNCALL strlen # get right len")
	emit("PUSH_8 # right len")

	emit("pop %%rax # right len")
	emit("pop %%rcx # right string")
	emit("pop %%rbx # left len")
	emit("pop %%rdx # left string")

	emit("push %%rcx # right string")
	emit("push %%rdx # left  string")

	// newSize = strlen(left) + strlen(right) + 1
	emit("add %%rax, %%rbx # len + len")
	emit("add $1, %%rbx # + 1 (null byte)")
	emit("mov %%rbx, %%rax")
	emit("PUSH_8")
	emit("POP_TO_ARG_0")
	emit("FUNCALL iruntime.malloc")

	emit("PUSH_8")
	emit("POP_TO_ARG_0")
	emit("POP_TO_ARG_1")
	emit("FUNCALL strcat")

	emit("PUSH_8")
	emit("POP_TO_ARG_0")
	emit("POP_TO_ARG_1")
	emit("FUNCALL strcat")
}

func (ast *ExprBinop) emit() {
	if ast.op == "+" && ast.left.getGtype().isString() {
		emitStringConcate(ast.left, ast.right)
		return
	}
	switch ast.op {
	case "<", ">", "<=", ">=", "!=", "==":
		ast.emitComp()
		return
	case "&&":
		labelEnd := makeLabel()
		ast.left.emit()
		emit("TEST_IT")
		emit("LOAD_NUMBER 0")
		emit("je %s", labelEnd)
		ast.right.emit()
		emit("TEST_IT")
		emit("LOAD_NUMBER 0")
		emit("je %s", labelEnd)
		emit("LOAD_NUMBER 1")
		emit("%s:", labelEnd)
		return
	case "||":
		labelEnd := makeLabel()
		ast.left.emit()
		emit("TEST_IT")
		emit("LOAD_NUMBER 1")
		emit("jne %s", labelEnd)
		ast.right.emit()
		emit("TEST_IT")
		emit("LOAD_NUMBER 1")
		emit("jne %s", labelEnd)
		emit("LOAD_NUMBER 0")
		emit("%s:", labelEnd)
		return
	}
	ast.left.emit()
	emit("PUSH_8")
	ast.right.emit()
	emit("PUSH_8")

	if ast.op == "+" {
		emit("SUM_FROM_STACK")
	} else if ast.op == "-" {
		emit("SUB_FROM_STACK")
	} else if ast.op == "*" {
		emit("IMUL_FROM_STACK")
	} else if ast.op == "%" {
		emit("pop %%rcx")
		emit("pop %%rax")
		emit("mov $0, %%rdx # init %%rdx")
		emit("div %%rcx")
		emit("mov %%rdx, %%rax")
	} else if ast.op == "/" {
		emit("pop %%rcx")
		emit("pop %%rax")
		emit("mov $0, %%rdx # init %%rdx")
		emit("div %%rcx")
	} else {
		errorft(ast.token(), "Unknown binop: %s", ast.op)
	}
}

func isUnderScore(e Expr) bool {
	rel, ok := e.(*Relation)
	if !ok {
		return false
	}
	return rel.name == "_"
}

// https://golang.org/ref/spec#Assignments
// A tuple assignment assigns the individual elements of a multi-valued operation to a list of variables.
// There are two forms.
//
// In the first,
// the right hand operand is a single multi-valued expression such as a function call, a channel or map operation, or a type assertion.
// The number of operands on the left hand side must match the number of values.
// For instance, if f is a function returning two values,
//
//	x, y = f()
//
// assigns the first value to x and the second to y.
//
// In the second form,
// the number of operands on the left must equal the number of expressions on the right,
// each of which must be single-valued, and the nth expression on the right is assigned to the nth operand on the left:
//
//  one, two, three = '一', '二', '三'
//
func (ast *StmtAssignment) emit() {
	emit("# StmtAssignment")
	// the right hand operand is a single multi-valued expression
	// such as a function call, a channel or map operation, or a type assertion.
	// The number of operands on the left hand side must match the number of values.
	isOnetoOneAssignment := (len(ast.rights) > 1)
	if isOnetoOneAssignment {
		emit("# multi(%d) = multi(%d)", len(ast.lefts), len(ast.rights))
		// a,b,c = expr1,expr2,expr3
		if len(ast.lefts) != len(ast.rights) {
			errorft(ast.token(), "number of exprs does not match")
		}

		for rightIndex, right := range ast.rights {
			left := ast.lefts[rightIndex]
			switch right.(type) {
			case *ExprFuncallOrConversion, *ExprMethodcall:
				rettypes := getRettypes(right)
				assert(len(rettypes) == 1, ast.token(), "return values should be one")
			}
			gtype := left.getGtype()
			switch {
			case gtype.getKind() == G_ARRAY:
				assignToArray(left, right)
			case gtype.getKind() == G_SLICE:
				assignToSlice(left, right)
			case gtype.getKind() == G_STRUCT:
				assignToStruct(left, right)
			case gtype.getKind() == G_INTERFACE:
				assignToInterface(left, right)
			default:
				// suppose primitive
				emitAssignPrimitive(left, right)
			}
		}
		return
	} else {
		numLeft := len(ast.lefts)
		emit("# multi(%d) = expr", numLeft)
		// a,b,c = expr
		numRight := 0
		right := ast.rights[0]

		var leftsMayBeTwo bool // a(,b) := expr // map index or type assertion
		switch right.(type) {
		case *ExprFuncallOrConversion, *ExprMethodcall:
			rettypes := getRettypes(right)
			if isOnetoOneAssignment && len(rettypes) > 1 {
				errorft(ast.token(), "multivalue is not allowed")
			}
			numRight += len(rettypes)
		case *ExprTypeAssertion:
			leftsMayBeTwo = true
			numRight++
		case *ExprIndex:
			indexExpr := right.(*ExprIndex)
			if indexExpr.collection.getGtype().getKind() == G_MAP {
				// map get
				emit("# v, ok = map[k]")
				leftsMayBeTwo = true
			}
			numRight++
		default:
			numRight++
		}

		if leftsMayBeTwo {
			if numLeft > 2 {
				errorft(ast.token(), "number of exprs does not match. numLeft=%d", numLeft)
			}
		} else {
			if numLeft != numRight {
				errorft(ast.token(), "number of exprs does not match: %d <=> %d", numLeft, numRight)
			}
		}

		left := ast.lefts[0]
		switch right.(type) {
		case *ExprFuncallOrConversion, *ExprMethodcall:
			rettypes := getRettypes(right)
			if len(rettypes) > 1 {
				// a,b,c = f()
				emit("# a,b,c = f()")
				right.emit()
				var retRegiLen int
				for _, rettype := range rettypes {
					retSize := rettype.getSize()
					if retSize < 8 {
						retSize = 8
					}
					retRegiLen += retSize / 8
				}
				emit("# retRegiLen=%d\n", retRegiLen)
				for i := retRegiLen - 1; i >= 0; i-- {
					emit("push %%%s # %d", retRegi[i], i)
				}
				for _, left := range ast.lefts {
					if isUnderScore(left) {
						continue
					}
					assert(left.getGtype() != nil, left.token(), "should not be nil")
					if left.getGtype().kind == G_SLICE {
						// @TODO: Does this work ?
						emitSave24(left, 0)
					} else if left.getGtype().getKind() == G_INTERFACE {
						// @TODO: Does this work ?
						emitSave24(left, 0)
					} else {
						emit("pop %%rax")
						emitSave(left)
					}
				}
				return
			}
		}

		gtype := left.getGtype()
		if _, ok := left.(*Relation); ok {
			emit("# \"%s\" = ", left.(*Relation).name)
		}
		//emit("# Assign %T %s = %T %s", left, gtype.String(), right, right.getGtype())
		switch {
		case gtype == nil:
			// suppose left is "_"
			right.emit()
		case gtype.getKind() == G_ARRAY:
			assignToArray(left, right)
		case gtype.getKind() == G_SLICE:
			assignToSlice(left, right)
		case gtype.getKind() == G_STRUCT:
			assignToStruct(left, right)
		case gtype.getKind() == G_INTERFACE:
			assignToInterface(left, right)
		case gtype.getKind() == G_MAP:
			assignToMap(left, right)
		default:
			// suppose primitive
			emitAssignPrimitive(left, right)
		}
		if leftsMayBeTwo && len(ast.lefts) == 2 {
			okVariable := ast.lefts[1]
			okRegister := mapOkRegister(right.getGtype().is24Width())
			emit("mov %%%s, %%rax # emit okValue", okRegister)
			emitSave(okVariable)
		}
		return
	}

}

func emitAssignPrimitive(left Expr, right Expr) {
	assert(left.getGtype().getSize() <= 8, left.token(), fmt.Sprintf("invalid type for lhs: %s", left.getGtype()))
	assert(right != nil || right.getGtype().getSize() <= 8, right.token(), fmt.Sprintf("invalid type for rhs: %s", right.getGtype()))
	right.emit()   //   expr => %rax
	emitSave(left) //   %rax => memory
}

// Each left-hand side operand must be addressable,
// a map index expression,
// or (for = assignments only) the blank identifier.
func emitSave(left Expr) {
	switch left.(type) {
	case *Relation:
		emit("# %s %s = ", left.(*Relation).name, left.getGtype().String())
		left.(*Relation).emitSave()
	case *ExprIndex:
		left.(*ExprIndex).emitSave()
	case *ExprStructField:
		left.(*ExprStructField).emitSave()
	case *ExprUop:
		left.(*ExprUop).emitSave()
	default:
		left.dump()
		errorft(left.token(), "Unknown case %T", left)
	}
}

// save data from stack
func (e *ExprIndex) emitSave24() {
	// load head address of the array
	// load index
	// multi index * size
	// calc address = head address + offset
	// copy value to the address

	collectionType := e.collection.getGtype()
	switch {
	case collectionType.getKind() == G_ARRAY, collectionType.getKind() == G_SLICE, collectionType.getKind() == G_STRING:
		e.collection.emit() // head address
	case collectionType.getKind() == G_MAP:
		e.emitMapSet(true)
		return
	default:
		TBI(e.token(), "unable to handle %s", collectionType)
	}
	emit("PUSH_8 # head address of collection")
	e.index.emit()
	emit("PUSH_8 # index")
	var elmType *Gtype
	if collectionType.isString() {
		elmType = gByte
	} else {
		elmType = collectionType.elementType
	}
	size := elmType.getSize()
	assert(size > 0, nil, "size > 0")
	emit("LOAD_NUMBER %d # elementSize", size)
	emit("PUSH_8")
	emit("IMUL_FROM_STACK # index * elementSize")
	emit("PUSH_8 # index * elementSize")
	emit("SUM_FROM_STACK # (index * size) + address")
	emit("PUSH_8")
	emit("STORE_24_INDIRECT_FROM_STACK")
}

func (e *ExprIndex) emitSave() {
	collectionType := e.collection.getGtype()
	switch {
	case collectionType.getKind() == G_ARRAY, collectionType.getKind() == G_SLICE, collectionType.getKind() == G_STRING:
		emitCollectIndexSave(e.collection, e.index, 0)
	case collectionType.getKind() == G_MAP:
		emit("PUSH_8") // push RHS value
		e.emitMapSet(false)
		return
	default:
		TBI(e.token(), "unable to handle %s", collectionType)
	}
}

func (e *ExprStructField) emitSave() {
	fieldType := e.getGtype()
	if e.strct.getGtype().kind == G_POINTER {
		emit("PUSH_8 # rhs")

		e.strct.emit()
		emit("ADD_NUMBER %d", fieldType.offset)
		emit("PUSH_8")

		emit("STORE_8_INDIRECT_FROM_STACK")
	} else {
		emitOffsetSave(e.strct, 8, fieldType.offset)
	}
}

func (e *ExprStructField) emitOffsetLoad(size int, offset int) {
	rel, ok := e.strct.(*Relation)
	assert(ok, e.tok, "should be *Relation")
	vr, ok := rel.expr.(*ExprVariable)
	assert(ok, e.tok, "should be *ExprVariable")
	assert(vr.gtype.kind == G_NAMED, e.tok, "expect G_NAMED, but got "+vr.gtype.String())
	field := vr.gtype.relation.gtype.getField(e.fieldname)
	vr.emitOffsetLoad(size, field.offset+offset)
}

func (stmt *StmtIf) emit() {
	emit("# if")
	if stmt.simplestmt != nil {
		stmt.simplestmt.emit()
	}
	stmt.cond.emit()
	emit("TEST_IT")
	if stmt.els != nil {
		labelElse := makeLabel()
		labelEndif := makeLabel()
		emit("je %s  # jump if 0", labelElse)
		emit("# then block")
		stmt.then.emit()
		emit("jmp %s # jump to endif", labelEndif)
		emit("# else block")
		emit("%s:", labelElse)
		stmt.els.emit()
		emit("# endif")
		emit("%s:", labelEndif)
	} else {
		// no else block
		labelEndif := makeLabel()
		emit("je %s  # jump if 0", labelEndif)
		emit("# then block")
		stmt.then.emit()
		emit("# endif")
		emit("%s:", labelEndif)
	}
}

func (stmt *StmtSwitch) emit() {

	emit("#")
	emit("# switch statement")
	labelEnd := makeLabel()
	var labels []string

	// switch (expr) {
	if stmt.cond != nil {
		emit("# the subject expression")
		stmt.cond.emit()
		emit("PUSH_8 # the subject value")
		emit("#")
	} else {
		// switch {
		emit("# no condition")
	}

	// case exp1,exp2,..:
	//     stmt1;
	//     stmt2;
	//     ...
	for i, caseClause := range stmt.cases {
		emit("# case %d", i)
		myCaseLabel := makeLabel()
		labels = append(labels, myCaseLabel)
		if stmt.cond == nil {
			for _, e := range caseClause.exprs {
				e.emit()
				emit("TEST_IT")
				emit("jne %s # jump if matches", myCaseLabel)
			}
		} else if stmt.isTypeSwitch {
			// compare type
			for _, gtype := range caseClause.gtypes {
				emit("# Duplicate the subject value in stack")
				emit("POP_8")
				emit("PUSH_8")
				emit("PUSH_8")

				if gtype.isNil() {
					emit("mov $0, %%rax # nil")
				} else {
					typeLabel := groot.getTypeLabel(gtype)
					emit("LOAD_STRING_LITERAL .%s # type: %s", typeLabel, gtype.String())
				}
				emit("PUSH_8")
				emitStringsEqualFromStack(true)

				emit("TEST_IT")
				emit("jne %s # jump if matches", myCaseLabel)
			}
		} else {
			for _, e := range caseClause.exprs {
				emit("# Duplicate the subject value in stack")
				emit("POP_8")
				emit("PUSH_8")
				emit("PUSH_8")

				e.emit()
				emit("PUSH_8")
				if e.getGtype().isString() {
					emitStringsEqualFromStack(true)
				} else {
					emit("CMP_FROM_STACK sete")
				}

				emit("TEST_IT")
				emit("jne %s # jump if matches", myCaseLabel)
			}
		}
	}

	var defaultLabel string
	if stmt.dflt == nil {
		emit("jmp %s", labelEnd)
	} else {
		emit("# default")
		defaultLabel = makeLabel()
		emit("jmp %s", defaultLabel)
	}

	emit("POP_8 # destroy the subject value")
	emit("#")
	for i, caseClause := range stmt.cases {
		emit("# case stmts")
		emit("%s:", labels[i])
		caseClause.compound.emit()
		emit("jmp %s", labelEnd)
	}

	if stmt.dflt != nil {
		emit("%s:", defaultLabel)
		stmt.dflt.emit()
	}

	emit("%s: # end of switch", labelEnd)
}

func (f *StmtFor) emitRangeForList() {
	emitNewline()
	emit("# for range %s", f.rng.rangeexpr.getGtype().String())
	assertNotNil(f.rng.indexvar != nil, f.rng.tok)
	assert(f.rng.rangeexpr.getGtype().kind == G_ARRAY || f.rng.rangeexpr.getGtype().kind == G_SLICE, f.rng.tok, "rangeexpr should be G_ARRAY or G_SLICE, but got "+f.rng.rangeexpr.getGtype().String())

	labelBegin := makeLabel()
	f.labelEndBlock = makeLabel()
	f.labelEndLoop = makeLabel()

	// i = 0
	emit("# init index")
	initstmt := &StmtAssignment{
		lefts: []Expr{
			f.rng.indexvar,
		},
		rights: []Expr{
			&ExprNumberLiteral{
				val: 0,
			},
		},
	}
	initstmt.emit()

	emit("%s: # begin loop ", labelBegin)

	// i < len(list)
	condition := &ExprBinop{
		op:   "<",
		left: f.rng.indexvar, // i
		// @TODO
		// The range expression x is evaluated once before beginning the loop
		right: &ExprLen{
			arg: f.rng.rangeexpr, // len(expr)
		},
	}
	condition.emit()
	emit("TEST_IT")
	emit("je %s  # if false, go to loop end", f.labelEndLoop)

	// v = s[i]
	var assignVar *StmtAssignment
	if f.rng.valuevar != nil {
		assignVar = &StmtAssignment{
			lefts: []Expr{
				f.rng.valuevar,
			},
			rights: []Expr{
				&ExprIndex{
					collection: f.rng.rangeexpr,
					index:      f.rng.indexvar,
				},
			},
		}
		assignVar.emit()
	}

	f.block.emit()
	emit("%s: # end block", f.labelEndBlock)

	// break if i == len(list) - 1
	condition2 := &ExprBinop{
		op:   "==",
		left: f.rng.indexvar, // i
		// @TODO2
		// The range expression x is evaluated once before beginning the loop
		right: &ExprBinop{
			op: "-",
			left: &ExprLen{
				arg: f.rng.rangeexpr, // len(expr)
			},
			right: &ExprNumberLiteral{
				val: 1,
			},
		},
	}
	condition2.emit()
	emit("TEST_IT")
	emit("jne %s  # if this iteration is final, go to loop end", f.labelEndLoop)

	// i++
	indexIncr := &StmtInc{
		operand: f.rng.indexvar,
	}
	indexIncr.emit()

	emit("jmp %s", labelBegin)
	emit("%s: # end loop", f.labelEndLoop)
}

func (f *StmtFor) emitForClause() {
	assertNotNil(f.cls != nil, nil)
	labelBegin := makeLabel()
	f.labelEndBlock = makeLabel()
	f.labelEndLoop = makeLabel()

	if f.cls.init != nil {
		f.cls.init.emit()
	}
	emit("%s: # begin loop ", labelBegin)
	if f.cls.cond != nil {
		f.cls.cond.emit()
		emit("TEST_IT")
		emit("je %s  # jump if false", f.labelEndLoop)
	}
	f.block.emit()
	emit("%s: # end block", f.labelEndBlock)
	if f.cls.post != nil {
		f.cls.post.emit()
	}
	emit("jmp %s", labelBegin)
	emit("%s: # end loop", f.labelEndLoop)
}

func (f *StmtFor) emit() {
	if f.rng != nil {
		if f.rng.rangeexpr.getGtype().getKind() == G_MAP {
			f.emitRangeForMap()
		} else {
			f.emitRangeForList()
		}
		return
	}
	f.emitForClause()
}

func (stmt *StmtReturn) emitDeferAndReturn() {
	if stmt.labelDeferHandler != "" {
		emit("# defer and return")
		emit("jmp %s", stmt.labelDeferHandler)
	}
}

// expect rhs address is in the stack top, lhs is in the second top
func emitCopyStructFromStack(size int) {
	emit("pop %%rbx") // to
	emit("pop %%rax") // from

	var i int
	for ; i < size; i += 8 {
		emit("movq %d(%%rbx), %%rcx", i)
		emit("movq %%rcx, %d(%%rax)", i)
	}
	for ; i < size; i += 4 {
		emit("movl %d(%%rbx), %%rcx", i)
		emit("movl %%rcx, %d(%%rax)", i)
	}
	for ; i < size; i++ {
		emit("movb %d(%%rbx), %%rcx", i)
		emit("movb %%rcx, %d(%%rax)", i)
	}
}

func assignToStruct(lhs Expr, rhs Expr) {
	emit("# assignToStruct start")

	if rel, ok := lhs.(*Relation); ok {
		lhs = rel.expr
	}
	assert(rhs == nil || (rhs.getGtype().kind == G_NAMED && rhs.getGtype().relation.gtype.kind == G_STRUCT),
		lhs.token(), "rhs should be struct type")
	// initializes with zero values
	emit("# initialize struct with zero values: start")
	for _, fieldtype := range lhs.getGtype().relation.gtype.fields {
		switch {
		case fieldtype.kind == G_ARRAY:
			arrayType := fieldtype
			elementType := arrayType.elementType
			elmSize := arrayType.elementType.getSize()
			switch {
			case elementType.kind == G_NAMED && elementType.relation.gtype.kind == G_STRUCT:
				left := &ExprStructField{
					strct:     lhs,
					fieldname: fieldtype.fieldname,
				}
				assignToArray(left, nil)
			default:
				assert(0 <= elmSize && elmSize <= 8, lhs.token(), "invalid size")
				for i := 0; i < arrayType.length; i++ {
					emit("mov $0, %%rax")
					emitOffsetSave(lhs, elmSize, fieldtype.offset+i*elmSize)
				}
			}

		case fieldtype.kind == G_SLICE:
			emit("LOAD_EMPTY_SLICE")
			emit("PUSH_SLICE")
			emitSave24(lhs, fieldtype.offset)
		case fieldtype.kind == G_MAP:
			emit("LOAD_EMPTY_MAP")
			emit("PUSH_MAP")
			emitSave24(lhs, fieldtype.offset)
		case fieldtype.kind == G_NAMED && fieldtype.relation.gtype.kind == G_STRUCT:
			left := &ExprStructField{
				strct:     lhs,
				fieldname: fieldtype.fieldname,
			}
			assignToStruct(left, nil)
		case fieldtype.getKind() == G_INTERFACE:
			emit("LOAD_EMPTY_INTERFACE")
			emit("PUSH_INTERFACE")
			emitSave24(lhs, fieldtype.offset)
		default:
			emit("mov $0, %%rax")
			regSize := fieldtype.getSize()
			assert(0 < regSize && regSize <= 8, lhs.token(), fieldtype.String())
			emitOffsetSave(lhs, regSize, fieldtype.offset)
		}
	}
	emit("# initialize struct with zero values: end")

	if rhs == nil {
		return
	}
	variable := lhs

	strcttyp := rhs.getGtype().Underlying()

	switch rhs.(type) {
	case *Relation:
		emitAddress(lhs)
		emit("PUSH_8")
		emitAddress(rhs)
		emit("PUSH_8")
		emitCopyStructFromStack(lhs.getGtype().getSize())
	case *ExprUop:
		re := rhs.(*ExprUop)
		if re.op == "*" {
			// copy struct
			emitAddress(lhs)
			emit("PUSH_8")
			re.operand.emit()
			emit("PUSH_8")
			emitCopyStructFromStack(lhs.getGtype().getSize())
		} else {
			TBI(rhs.token(), "")
		}
	case *ExprStructLiteral:
		structliteral, ok := rhs.(*ExprStructLiteral)
		assert(ok || rhs == nil, rhs.token(), fmt.Sprintf("invalid rhs: %T", rhs))

		// do assignment for each field
		for _, field := range structliteral.fields {
			emit("# .%s", field.key)
			fieldtype := strcttyp.getField(field.key)

			switch {
			case fieldtype.kind == G_ARRAY:
				initvalues, ok := field.value.(*ExprArrayLiteral)
				assert(ok, nil, "ok")
				arrayType := strcttyp.getField(field.key)
				elementType := arrayType.elementType
				elmSize := elementType.getSize()
				switch {
				case elementType.kind == G_NAMED && elementType.relation.gtype.kind == G_STRUCT:
					left := &ExprStructField{
						strct:     lhs,
						fieldname: fieldtype.fieldname,
					}
					assignToArray(left, field.value)
				default:
					for i, val := range initvalues.values {
						val.emit()
						emitOffsetSave(variable, elmSize, arrayType.offset+i*elmSize)
					}
				}
			case fieldtype.kind == G_SLICE:
				left := &ExprStructField{
					tok:       variable.token(),
					strct:     lhs,
					fieldname: field.key,
				}
				assignToSlice(left, field.value)
			case fieldtype.getKind() == G_MAP:
				left := &ExprStructField{
					tok:       variable.token(),
					strct:     lhs,
					fieldname: field.key,
				}
				assignToMap(left, field.value)
			case fieldtype.getKind() == G_INTERFACE:
				left := &ExprStructField{
					tok:       lhs.token(),
					strct:     lhs,
					fieldname: field.key,
				}
				assignToInterface(left, field.value)
			case fieldtype.kind == G_NAMED && fieldtype.relation.gtype.kind == G_STRUCT:
				left := &ExprStructField{
					tok:       variable.token(),
					strct:     lhs,
					fieldname: field.key,
				}
				assignToStruct(left, field.value)
			default:
				field.value.emit()

				regSize := fieldtype.getSize()
				assert(0 < regSize && regSize <= 8, variable.token(), fieldtype.String())
				emitOffsetSave(variable, regSize, fieldtype.offset)
			}
		}
	default:
		TBI(rhs.token(), "")
	}

	emit("# assignToStruct end")
}

const sliceOffsetForLen = 8

func emitOffsetSave(lhs Expr, size int, offset int) {
	switch lhs.(type) {
	case *Relation:
		rel := lhs.(*Relation)
		assert(rel.expr != nil, rel.token(), "left.rel.expr is nil")
		emitOffsetSave(rel.expr, size, offset)
	case *ExprVariable:
		variable := lhs.(*ExprVariable)
		variable.emitOffsetSave(size, offset, false)
	case *ExprStructField:
		structfield := lhs.(*ExprStructField)
		fieldType := structfield.getGtype()
		emitOffsetSave(structfield.strct, size, fieldType.offset+offset)
	case *ExprIndex:
		indexExpr := lhs.(*ExprIndex)
		emitCollectIndexSave(indexExpr.collection, indexExpr.index, offset)

	default:
		errorft(lhs.token(), "unkonwn type %T", lhs)
	}
}

// take slice values from stack
func emitSave24(lhs Expr, offset int) {
	assertInterface(lhs)
	//emit("# emitSave24(%T, offset %d)", lhs, offset)
	emit("# emitSave24(?, offset %d)", offset)
	switch lhs.(type) {
	case *Relation:
		rel := lhs.(*Relation)
		emitSave24(rel.expr, offset)
	case *ExprVariable:
		variable := lhs.(*ExprVariable)
		variable.emitSave24(offset)
	case *ExprStructField:
		structfield := lhs.(*ExprStructField)
		fieldType := structfield.getGtype()
		fieldOffset := fieldType.offset
		emit("# fieldOffset=%d (%s)", fieldOffset, fieldType.fieldname)
		emitSave24(structfield.strct, fieldOffset+offset)
	case *ExprIndex:
		indexExpr := lhs.(*ExprIndex)
		indexExpr.emitSave24()
	default:
		errorft(lhs.token(), "unkonwn type %T", lhs)
	}
}

func emitCallMallocDinamicSize(eSize Expr) {
	eSize.emit()
	emit("PUSH_8")
	emit("POP_TO_ARG_0")
	emit("FUNCALL iruntime.malloc")
}

func emitCallMalloc(size int) {
	eNumber := &ExprNumberLiteral{
		val: size,
	}
	emitCallMallocDinamicSize(eNumber)
}

func assignToMap(lhs Expr, rhs Expr) {
	emit("# assignToMap")
	if rhs == nil {
		emit("# initialize map with a zero value")
		emit("LOAD_EMPTY_MAP")
		emit("PUSH_MAP")
		emitSave24(lhs, 0)
		return
	}
	switch rhs.(type) {
	case *ExprMapLiteral:
		emit("# map literal")

		lit := rhs.(*ExprMapLiteral)
		lit.emit()
		emit("PUSH_MAP")
	case *Relation, *ExprVariable, *ExprIndex, *ExprStructField, *ExprFuncallOrConversion, *ExprMethodcall:
		rhs.emit()
		emit("PUSH_MAP")
	default:
		TBI(rhs.token(), "unable to handle %T", rhs)
	}
	emitSave24(lhs, 0)
}

func (e *ExprConversionToInterface) emit() {
	emit("# ExprConversionToInterface")
	emitConversionToInterface(e.expr)
}

func emitConversionToInterface(dynamicValue Expr) {
	receiverType := dynamicValue.getGtype()
	if receiverType == nil {
		emit("# receiverType is nil. emit nil for interface")
		emit("LOAD_EMPTY_INTERFACE")
		return
	}

	emit("# emitConversionToInterface from %s", dynamicValue.getGtype().String())
	dynamicValue.emit()
	emit("PUSH_8")
	emitCallMalloc(8)
	emit("PUSH_8")
	emit("STORE_8_INDIRECT_FROM_STACK")
	emit("PUSH_8 # addr of dynamicValue") // address

	if receiverType.kind == G_POINTER {
		receiverType = receiverType.origType.relation.gtype
	}
	//assert(receiverType.receiverTypeId > 0,  dynamicValue.token(), "no receiverTypeId")
	emit("LOAD_NUMBER %d # receiverTypeId", receiverType.receiverTypeId)
	emit("PUSH_8 # receiverTypeId")

	gtype := dynamicValue.getGtype()
	label := groot.getTypeLabel(gtype)
	emit("lea .%s, %%rax# dynamicType %s", label, gtype.String())
	emit("PUSH_8 # dynamicType")

	emit("POP_INTERFACE")
	emitNewline()
}

func isNil(e Expr) bool {
	rel, ok := e.(*Relation)
	if ok {
		_, isNil := rel.expr.(*ExprNilLiteral)
		return isNil
	}
	return false
}


func assignToInterface(lhs Expr, rhs Expr) {
	emit("# assignToInterface")
	if rhs == nil || isNil(rhs) {
		emit("LOAD_EMPTY_INTERFACE")
		emit("PUSH_INTERFACE")
		emitSave24(lhs, 0)
		return
	}

	assert(rhs.getGtype() != nil, rhs.token(), fmt.Sprintf("rhs gtype is nil:%T", rhs))
	if rhs.getGtype().getKind() == G_INTERFACE {
		rhs.emit()
		emit("PUSH_INTERFACE")
		emitSave24(lhs, 0)
		return
	}

	emitConversionToInterface(rhs)
	emit("PUSH_INTERFACE")
	emitSave24(lhs, 0)
}

func assignToSlice(lhs Expr, rhs Expr) {
	emit("# assignToSlice")
	assertInterface(lhs)
	//assert(rhs == nil || rhs.getGtype().kind == G_SLICE, nil, "should be a slice literal or nil")
	if rhs == nil {
		emit("LOAD_EMPTY_SLICE")
		emit("PUSH_SLICE")
		emitSave24(lhs, 0)
		return
	}

	//	assert(rhs.getGtype().getKind() == G_SLICE, rhs.token(), "rsh should be slice type")

	switch rhs.(type) {
	case *Relation:
		rel := rhs.(*Relation)
		if _, ok := rel.expr.(*ExprNilLiteral); ok {
			emit("LOAD_EMPTY_SLICE")
			emit("PUSH_SLICE")
			emitSave24(lhs, 0)
			return
		}
		rvariable, ok := rel.expr.(*ExprVariable)
		assert(ok, nil, "ok")
		rvariable.emit()
		emit("PUSH_SLICE")
	case *ExprSliceLiteral:
		lit := rhs.(*ExprSliceLiteral)
		lit.emit()
		emit("PUSH_SLICE")
	case *ExprSlice:
		e := rhs.(*ExprSlice)
		e.emit()
		emit("PUSH_SLICE")
	case *ExprConversion:
		// https://golang.org/ref/spec#Conversions
		// Converting a value of a string type to a slice of bytes type
		// yields a slice whose successive elements are the bytes of the string.
		//
		// see also https://blog.golang.org/strings
		conversion := rhs.(*ExprConversion)
		assert(conversion.gtype.kind == G_SLICE, rhs.token(), "must be a slice of bytes")
		assert(conversion.expr.getGtype().kind == G_STRING || conversion.expr.getGtype().relation.gtype.kind == G_STRING, rhs.token(), "must be a string type, but got "+conversion.expr.getGtype().String())
		stringVarname, ok := conversion.expr.(*Relation)
		assert(ok, rhs.token(), "ok")
		stringVariable := stringVarname.expr.(*ExprVariable)
		stringVariable.emit()
		emit("PUSH_8 # ptr")
		strlen := &ExprLen{
			arg: stringVariable,
		}
		strlen.emit()
		emit("PUSH_8 # len")
		emit("PUSH_8 # cap")

	default:
		//emit("# emit rhs of type %T %s", rhs, rhs.getGtype().String())
		rhs.emit() // it should put values to rax,rbx,rcx
		emit("PUSH_SLICE")
	}

	emitSave24(lhs, 0)
}

func (variable *ExprVariable) emitSave24(offset int) {
	emit("# *ExprVariable.emitSave24()")
	emit("pop %%rax # 3rd")
	variable.emitOffsetSave(8, offset+16, false)
	emit("pop %%rax # 2nd")
	variable.emitOffsetSave(8, offset+8, false)
	emit("pop %%rax # 1st")
	variable.emitOffsetSave(8, offset+0, true)
}

// copy each element
func assignToArray(lhs Expr, rhs Expr) {
	emit("# assignToArray")
	if rel, ok := lhs.(*Relation); ok {
		lhs = rel.expr
	}

	arrayType := lhs.getGtype()
	elementType := arrayType.elementType
	elmSize := elementType.getSize()
	assert(rhs == nil || rhs.getGtype().kind == G_ARRAY, nil, "rhs should be array")
	switch {
	case elementType.kind == G_NAMED && elementType.relation.gtype.kind == G_STRUCT:
		//TBI
		for i := 0; i < arrayType.length; i++ {
			left := &ExprIndex{
				collection: lhs,
				index:      &ExprNumberLiteral{val: i},
			}
			if rhs == nil {
				assignToStruct(left, nil)
				continue
			}
			arrayLiteral, ok := rhs.(*ExprArrayLiteral)
			assert(ok, nil, "ok")
			assignToStruct(left, arrayLiteral.values[i])
		}
		return
	default: // prrimitive type or interface
		for i := 0; i < arrayType.length; i++ {
			offsetByIndex := i * elmSize
			switch rhs.(type) {
			case nil:
				// assign zero values
				if elementType.getKind() == G_INTERFACE {
					emit("LOAD_EMPTY_INTERFACE")
					emit("PUSH_INTERFACE")
					emitSave24(lhs, offsetByIndex)
					continue
				} else {
					emit("mov $0, %%rax")
				}
			case *ExprArrayLiteral:
				arrayLiteral := rhs.(*ExprArrayLiteral)
				if elementType.getKind() == G_INTERFACE {
					if i >= len(arrayLiteral.values) {
						// zero value
						emit("LOAD_EMPTY_INTERFACE")
						emit("PUSH_INTERFACE")
						emitSave24(lhs, offsetByIndex)
						continue
					} else if arrayLiteral.values[i].getGtype().getKind() != G_INTERFACE {
						// conversion of dynamic type => interface type
						dynamicValue := arrayLiteral.values[i]
						emitConversionToInterface(dynamicValue)
						emit("LOAD_EMPTY_INTERFACE")
						emit("PUSH_INTERFACE")
						emitSave24(lhs, offsetByIndex)
						continue
					} else {
						arrayLiteral.values[i].emit()
						emitSave24(lhs, offsetByIndex)
						continue
					}
				}

				if i >= len(arrayLiteral.values) {
					// zero value
					emit("mov $0, %%rax")
				} else {
					val := arrayLiteral.values[i]
					val.emit()
				}
			case *Relation:
				rel := rhs.(*Relation)
				arrayVariable, ok := rel.expr.(*ExprVariable)
				assert(ok, nil, "ok")
				arrayVariable.emitOffsetLoad(elmSize, offsetByIndex)
			case *ExprStructField:
				strctField := rhs.(*ExprStructField)
				strctField.emitOffsetLoad(elmSize, offsetByIndex)
			default:
				TBI(rhs.token(), "no supporetd %T", rhs)
			}

			emitOffsetSave(lhs, elmSize, offsetByIndex)
		}
	}
}

func (decl *DeclVar) emit() {
	if decl.variable.isGlobal {
		decl.emitGlobal()
	} else {
		decl.emitLocal()
	}
}

func (decl *DeclVar) emitLocal() {
	emit("# DeclVar \"%s\"", decl.variable.varname)
	gtype := decl.variable.gtype
	varname := decl.varname
	switch {
	case gtype.kind == G_ARRAY:
		assignToArray(varname, decl.initval)
	case gtype.kind == G_SLICE:
		assignToSlice(varname, decl.initval)
	case gtype.kind == G_NAMED && gtype.relation.gtype.kind == G_STRUCT:
		assignToStruct(varname, decl.initval)
	case gtype.getKind() == G_MAP:
		assignToMap(varname, decl.initval)
	case gtype.getKind() == G_INTERFACE:
		assignToInterface(varname, decl.initval)
	default:
		assert(decl.variable.getGtype().getSize() <= 8, decl.token(), "invalid type:"+gtype.String())
		// primitive types like int,bool,byte
		rhs := decl.initval
		if rhs == nil {
			if gtype.isString() {
				rhs = &eEmptyString
			} else {
				// assign zero value
				rhs = &ExprNumberLiteral{}
			}
		}
		emit("# LOAD RHS")
		gasIndentLevel++
		rhs.emit()
		gasIndentLevel--
		comment := "initialize " + string(decl.variable.varname)
		emit("# Assign to LHS")
		gasIndentLevel++
		emit("STORE_%d_TO_LOCAL %d # %s",
			decl.variable.getGtype().getSize(), decl.variable.offset, comment)
		gasIndentLevel--
	}
}

var eEmptyString = ExprStringLiteral{
	val: "",
}

func (decl *DeclType) emit() {
	// nothing to do
}

func (decl *DeclConst) emit() {
	// nothing to do
}

func (ast *StmtSatementList) emit() {
	for _, stmt := range ast.stmts {
		emit("# Statement")
		gasIndentLevel++
		stmt.emit()
		gasIndentLevel--
	}
}

func emitCollectIndexSave(collection Expr, index Expr, offset int) {
	collectionType := collection.getGtype()
	assert(collectionType.getKind() == G_ARRAY ||collectionType.getKind() == G_SLICE || collectionType.getKind() == G_STRING, collection.token(), "should be collection")

	var elmType *Gtype
	if collectionType.isString() {
		elmType = gByte
	} else {
		elmType = collectionType.elementType
	}
	elmSize := elmType.getSize()
	assert(elmSize > 0, nil, "elmSize > 0")

	emit("PUSH_8 # rhs")

	collection.emit()
	emit("PUSH_8 # addr")

	index.emit()
	emit("IMUL_NUMBER %d # index * elmSize", elmSize)
	emit("PUSH_8")

	emit("SUM_FROM_STACK # (index * elmSize) + addr")
	emit("ADD_NUMBER %d # offset", offset)
	emit("PUSH_8")

	if elmSize == 1 {
		emit("STORE_1_INDIRECT_FROM_STACK")
	} else {
		emit("STORE_8_INDIRECT_FROM_STACK")
	}
	emitNewline()
}

func emitEmptyString() {
	eEmpty := &eEmptyString
	eEmpty.emit()
}

func (e *ExprIndex) emit() {
	emit("# emit *ExprIndex")
	loadCollectIndex(e.collection, e.index, 0)
}

func (e *ExprNilLiteral) emit() {
	emit("LOAD_NUMBER 0 # nil literal")
}

func (ast *StmtShortVarDecl) emit() {
	a := &StmtAssignment{
		tok:    ast.tok,
		lefts:  ast.lefts,
		rights: ast.rights,
	}
	a.emit()
}

func (f *ExprFuncRef) emit() {
	emit("LOAD_NUMBER 1 # funcref") // emit 1 for now.  @FIXME
}

func (e ExprArrayLiteral) emit() {
	errorft(e.token(), "DO NOT EMIT")
}

// https://golang.org/ref/spec#Type_assertions
func (e *ExprTypeAssertion) emit() {
	assert(e.expr.getGtype().getKind() == G_INTERFACE, e.token(), "expr must be an Interface type")
	if e.gtype.getKind() == G_INTERFACE {
		TBI(e.token(), "")
	} else {
		// if T is not an interface type,
		// x.(T) asserts that the dynamic type of x is identical to the type T.

		e.expr.emit() // emit interface
		// rax(ptr), rbx(receiverTypeId of method table), rcx(hashed receiverTypeId)
		emit("PUSH_8")
		// @TODO DRY with type switch statement
		typeLabel := groot.getTypeLabel(e.gtype)
		emit("lea .%s(%%rip), %%rax # type: %s", typeLabel, e.gtype.String())

		emit("push %%rcx") // @TODO ????
		emit("PUSH_8")
		emitStringsEqualFromStack(true)

		emit("mov %%rax, %%rbx") // move flag @TODO: this is BUG in slice,map cases
		// @TODO consider big data like slice, struct, etd
		emit("pop %%rax # load ptr")
		emit("TEST_IT")
		labelEnd := makeLabel()
		emit("je %s # jmp if nil", labelEnd)
		emit("LOAD_8_BY_DEREF")
		emitWithoutIndent("%s:", labelEnd)
	}
}

func (ast *StmtContinue) emit() {
	assert(ast.stmtFor.labelEndBlock != "", ast.token(), "labelEndLoop should not be empty")
	emit("jmp %s # continue", ast.stmtFor.labelEndBlock)
}

func (ast *StmtBreak) emit() {
	assert(ast.stmtFor.labelEndLoop != "", ast.token(), "labelEndLoop should not be empty")
	emit("jmp %s # break", ast.stmtFor.labelEndLoop)
}

func (ast *StmtExpr) emit() {
	ast.expr.emit()
}

func (ast *StmtDefer) emit() {
	emit("# defer")
	/*
		// arguments should be evaluated immediately
		var args []Expr
		switch ast.expr.(type) {
		case *ExprMethodcall:
			call := ast.expr.(*ExprMethodcall)
			args = call.args
		case *ExprFuncallOrConversion:
			call := ast.expr.(*ExprFuncallOrConversion)
			args = call.args
		default:
			errorft(ast.token(), "defer should be a funcall")
		}
	*/
	labelStart := makeLabel() + "_defer"
	labelEnd := makeLabel() + "_defer"
	ast.label = labelStart

	emit("jmp %s", labelEnd)
	emit("%s: # defer start", labelStart)

	for i := 0; i < len(retRegi); i++ {
		emit("push %%%s", retRegi[i])
	}

	ast.expr.emit()

	for i := len(retRegi) - 1; i >= 0; i-- {
		emit("pop %%%s", retRegi[i])
	}

	emit("leave")
	emit("ret")
	emit("%s: # defer end", labelEnd)

}

func (e *ExprVaArg) emit() {
	e.expr.emit()
}

func (e *ExprConversion) emit() {
	emit("# ExprConversion.emit()")
	if e.gtype.isString() {
		// s = string(bytes)
		labelEnd := makeLabel()
		e.expr.emit()
		emit("TEST_IT")
		emit("jne %s", labelEnd)
		emitEmptyString()
		emit("%s:", labelEnd)
	} else {
		e.expr.emit()
	}
}

func (e *ExprStructLiteral) emit() {
	errorft(e.token(), "This cannot be emitted alone")
}

func (e *ExprTypeSwitchGuard) emit() {
	e.expr.emit()
	emit("mov %%rcx, %%rax # copy type id")
}

func (ast *ExprMethodcall) getUniqueName() string {
	gtype := ast.receiver.getGtype()
	return getMethodUniqueName(gtype, ast.fname)
}

func (methodCall *ExprMethodcall) getOrigType() *Gtype {
	gtype := methodCall.receiver.getGtype()
	assertNotNil(methodCall.receiver != nil, methodCall.token())
	assertNotNil(gtype != nil, methodCall.tok)
	assert(gtype.kind == G_NAMED || gtype.kind == G_POINTER || gtype.kind == G_INTERFACE, methodCall.tok, "method must be an interface or belong to a named type")
	var typeToBeloing *Gtype
	if gtype.kind == G_POINTER {
		typeToBeloing = gtype.origType
		assert(typeToBeloing != nil, methodCall.token(), "shoudl not be nil:"+gtype.String())
	} else {
		typeToBeloing = gtype
	}
	assert(typeToBeloing.kind == G_NAMED, methodCall.tok, "method must belong to a named type")
	origType := typeToBeloing.relation.gtype
	assert(typeToBeloing.relation.gtype != nil, methodCall.token(), fmt.Sprintf("origType should not be nil:%#v", typeToBeloing.relation))
	return origType
}

func getRettypes(call Expr) []*Gtype {
	switch call.(type) {
	case *ExprFuncallOrConversion:
		return call.(*ExprFuncallOrConversion).getRettypes()
	case *ExprMethodcall:
		return call.(*ExprMethodcall).getRettypes()
	}
	errorf("no reach here")
	return nil
}

func (funcall *ExprFuncallOrConversion) getRettypes() []*Gtype {
	if funcall.rel.gtype != nil {
		// Conversion
		return []*Gtype{funcall.rel.gtype}
	}

	return funcall.getFuncDef().rettypes
}

func (methodCall *ExprMethodcall) getRettypes() []*Gtype {
	origType := methodCall.getOrigType()
	if origType == nil {
		errorft(methodCall.token(), "origType should not be nil")
	}
	if origType.kind == G_INTERFACE {
		return origType.imethods[methodCall.fname].rettypes
	} else {
		funcref, ok := origType.methods[methodCall.fname]
		if !ok {
			errorft(methodCall.token(), "method %s is not found in type %s", methodCall.fname, methodCall.receiver.getGtype().String())
		}
		return funcref.funcdef.rettypes
	}
}

type IrInterfaceMethodCall struct {
	receiver   Expr
	methodName identifier
}

func (methodCall *ExprMethodcall) emitInterfaceMethodCall() {
	args := []Expr{methodCall.receiver}
	for _, arg := range methodCall.args {
		args = append(args, arg)
	}
	call := &IrInterfaceMethodCall{
		receiver:   methodCall.receiver,
		methodName: methodCall.fname,
	}
	call.emit(args)
}

func (methodCall *ExprMethodcall) emit() {
	origType := methodCall.getOrigType()
	if origType.kind == G_INTERFACE {
		methodCall.emitInterfaceMethodCall()
		return
	}

	args := []Expr{methodCall.receiver}
	for _, arg := range methodCall.args {
		args = append(args, arg)
	}

	funcref, ok := origType.methods[methodCall.fname]
	if !ok {
		errorft(methodCall.token(), "method %s is not found in type %s", methodCall.fname, methodCall.receiver.getGtype().String())
	}
	pkgname := funcref.funcdef.pkg
	name := methodCall.getUniqueName()
	var staticCall *IrStaticCall = &IrStaticCall{
		symbol:       getFuncSymbol(pkgname, name),
		callee:       funcref.funcdef,
		isMethodCall: true,
	}
	staticCall.emit(args)
}

func (funcall *ExprFuncallOrConversion) getFuncDef() *DeclFunc {
	relexpr := funcall.rel.expr
	assert(relexpr != nil, funcall.token(), fmt.Sprintf("relexpr should NOT be nil for %s", funcall.fname))
	funcref, ok := relexpr.(*ExprFuncRef)
	if !ok {
		errorft(funcall.token(), "Compiler error: funcref is not *ExprFuncRef (%s)", funcall.fname)
	}
	assertNotNil(funcref.funcdef != nil, nil)
	return funcref.funcdef
}

func (e *ExprLen) emit() {
	emit("# emit len()")
	arg := e.arg
	gtype := arg.getGtype()
	assert(gtype != nil, e.token(), "gtype should not be  nil:\n"+fmt.Sprintf("%#v", arg))

	switch {
	case gtype.kind == G_ARRAY:
		emit("LOAD_NUMBER %d", gtype.length)
	case gtype.kind == G_SLICE:
		emit("# len(slice)")
		switch arg.(type) {
		case *Relation:
			emit("# Relation")
			emitOffsetLoad(arg, 8, ptrSize)
		case *ExprStructField:
			emit("# ExprStructField")
			emitOffsetLoad(arg, 8, ptrSize)
		case *ExprIndex:
			emitOffsetLoad(arg, 8, ptrSize)
		case *ExprSliceLiteral:
			emit("# ExprSliceLiteral")
			_arg := arg.(*ExprSliceLiteral)
			length := len(_arg.values)
			emit("LOAD_NUMBER %d", length)
		case *ExprSlice:
			sliceExpr := arg.(*ExprSlice)
			uop := &ExprBinop{
				op:    "-",
				left:  sliceExpr.high,
				right: sliceExpr.low,
			}
			uop.emit()
		default:
			TBI(arg.token(), "unable to handle %T", arg)
		}
	case gtype.getKind() == G_MAP:
		emit("# emit len(map)")
		switch arg.(type) {
		case *Relation:
			emit("# Relation")
			emitOffsetLoad(arg, 8, ptrSize)
		case *ExprStructField:
			emit("# ExprStructField")
			emitOffsetLoad(arg, 8, ptrSize)
		case *ExprMapLiteral:
			TBI(arg.token(), "unable to handle %T", arg)
		default:
			TBI(arg.token(), "unable to handle %T", arg)
		}
	case gtype.getKind() == G_STRING:
		arg.emit()
		emit("PUSH_8")
		emit("POP_TO_ARG_0")
		emit("FUNCALL strlen")
	default:
		TBI(arg.token(), "unable to handle %s", gtype)
	}
}

func (e *ExprCap) emit() {
	emit("# emit cap()")
	arg := e.arg
	gtype := arg.getGtype()
	switch {
	case gtype.kind == G_ARRAY:
		emit("LOAD_NUMBER %d", gtype.length)
	case gtype.kind == G_SLICE:
		switch arg.(type) {
		case *Relation:
			emit("# Relation")
			emitOffsetLoad(arg, 8, ptrSize*2)
		case *ExprStructField:
			emit("# ExprStructField")
			emitOffsetLoad(arg, 8, ptrSize*2)
		case *ExprIndex:
			emitOffsetLoad(arg, 8, ptrSize*2)
		case *ExprSliceLiteral:
			emit("# ExprSliceLiteral")
			_arg := arg.(*ExprSliceLiteral)
			length := len(_arg.values)
			emit("LOAD_NUMBER %d", length)
		case *ExprSlice:
			sliceExpr := arg.(*ExprSlice)
			if sliceExpr.collection.getGtype().kind == G_ARRAY {
				cp := &ExprBinop{
					tok: e.tok,
					op:  "-",
					left: &ExprLen{
						tok: e.tok,
						arg: sliceExpr.collection,
					},
					right: sliceExpr.low,
				}
				cp.emit()
			} else {
				TBI(arg.token(), "unable to handle %T", arg)
			}
		default:
			TBI(arg.token(), "unable to handle %T", arg)
		}
	case gtype.getKind() == G_MAP:
		TBI(arg.token(), "unable to handle %T", arg)
	case gtype.getKind() == G_STRING:
		TBI(arg.token(), "unable to handle %T", arg)
	default:
		TBI(arg.token(), "unable to handle %s", gtype)
	}
}

func (funcall *ExprFuncallOrConversion) emit() {
	if funcall.rel.expr == nil && funcall.rel.gtype != nil {
		// Conversion
		conversion := &ExprConversion{
			tok:   funcall.token(),
			gtype: funcall.rel.gtype,
			expr:  funcall.args[0],
		}
		conversion.emit()
		return
	}

	assert(funcall.rel.expr != nil && funcall.rel.gtype == nil, funcall.token(), "this is conversion")
	assert(funcall.getFuncDef() != nil, funcall.token(), "funcdef is nil")
	decl := funcall.getFuncDef()

	// check if it's a builtin function
	switch decl {
	case builtinLen:
		assert(len(funcall.args) == 1, funcall.token(), "invalid arguments for len()")
		arg := funcall.args[0]
		exprLen := &ExprLen{
			tok: arg.token(),
			arg: arg,
		}
		exprLen.emit()
	case builtinCap:
		arg := funcall.args[0]
		e := &ExprCap{
			tok: arg.token(),
			arg: arg,
		}
		e.emit()
	case builtinAppend:
		assert(len(funcall.args) == 2, funcall.token(), "append() should take 2 argments")
		slice := funcall.args[0]
		valueToAppend := funcall.args[1]
		emit("# append(%s, %s)", slice.getGtype().String(), valueToAppend.getGtype().String())
		var staticCall *IrStaticCall = &IrStaticCall{
			callee: decl,
		}
		switch slice.getGtype().elementType.getSize() {
		case 1:
			staticCall.symbol = getFuncSymbol("iruntime", "append1")
			staticCall.emit(funcall.args)
		case 8:
			staticCall.symbol = getFuncSymbol("iruntime", "append8")
			staticCall.emit(funcall.args)
		case 24:
			if slice.getGtype().elementType.getKind() == G_INTERFACE && valueToAppend.getGtype().getKind() != G_INTERFACE {
				eConvertion := &ExprConversionToInterface{
					tok:  valueToAppend.token(),
					expr: valueToAppend,
				}
				funcall.args[1] = eConvertion
			}
			staticCall.symbol = getFuncSymbol("iruntime", "append24")
			staticCall.emit(funcall.args)
		default:
			TBI(slice.token(), "")
		}
	case builtinMakeSlice:
		assert(len(funcall.args) == 3, funcall.token(), "append() should take 3 argments")
		var staticCall *IrStaticCall = &IrStaticCall{
			callee: decl,
		}
		staticCall.symbol = getFuncSymbol("iruntime", "makeSlice")
		staticCall.emit(funcall.args)
	case builtinDumpSlice:
		arg := funcall.args[0]

		emit("lea .%s, %%rax", builtinStringKey2)
		emit("PUSH_8")

		arg.emit()
		emit("PUSH_SLICE")

		numRegs := 4
		for i := numRegs - 1; i >= 0; i-- {
			emit("POP_TO_ARG_%d", i)
		}

		emit("FUNCALL %s", "printf")
		emitNewline()
	case builtinDumpInterface:
		arg := funcall.args[0]

		emit("lea .%s, %%rax", builtinStringKey1)
		emit("PUSH_8")

		arg.emit()
		emit("PUSH_INTERFACE")

		numRegs := 4
		for i := numRegs - 1; i >= 0; i-- {
			emit("POP_TO_ARG_%d", i)
		}

		emit("FUNCALL %s", "printf")
		emitNewline()
	case builtinAssertInterface:
		emit("# builtinAssertInterface")
		labelEnd := makeLabel()
		arg := funcall.args[0]
		arg.emit() // rax=ptr, rbx=receverTypeId, rcx=dynamicTypeId

		// (ptr != nil && rcx == nil) => Error

		emit("CMP_NE_ZERO")
		emit("TEST_IT")
		emit("je %s", labelEnd)

		emit("mov %%rcx, %%rax")

		emit("CMP_EQ_ZERO")
		emit("TEST_IT")
		emit("je %s", labelEnd)

		slabel := makeLabel()
		emit(".data 0")
		emitWithoutIndent("%s:", slabel)
		emit(".string \"%s\"", "assertInterface failed")
		emit(".text")
		emit("lea %s, %%rax", slabel)
		emit("PUSH_8")
		emit("POP_TO_ARG_0")
		emit("FUNCALL %s", ".panic")

		emitWithoutIndent("%s:", labelEnd)
		emitNewline()

	case builtinAsComment:
		arg := funcall.args[0]
		if stringLiteral, ok := arg.(*ExprStringLiteral); ok {
			emitWithoutIndent("# %s", stringLiteral.val)
		}
	default:
		var staticCall *IrStaticCall = &IrStaticCall{
			symbol: getFuncSymbol(decl.pkg, funcall.fname),
			callee: decl,
		}
		staticCall.emit(funcall.args)
	}
}

type IrStaticCall struct {
	// https://sourceware.org/binutils/docs-2.30/as/Symbol-Intro.html#Symbol-Intro
	// A symbol is one or more characters chosen from the set of all letters (both upper and lower case), digits and the three characters ‘_.$’.
	symbol       string
	callee       *DeclFunc
	isMethodCall bool
}

func bool2string(bol bool) string {
	if bol {
		return "true"
	} else {
		return "false"
	}
}

func emitMakeSliceFunc() {
	// makeSlice
	emitWithoutIndent("%s:", "iruntime.makeSlice")
	emit("FUNC_PROLOGUE")
	emitNewline()

	emit("PUSH_ARG_2") // -8
	emit("PUSH_ARG_1") // -16
	emit("PUSH_ARG_0") // -24

	emit("mov -16(%%rbp), %%rax # newcap")
	emit("mov -8(%%rbp), %%rcx # unit")
	emit("imul %%rcx, %%rax")
	emit("ADD_NUMBER 16 # pure buffer")

	emit("PUSH_8")
	emit("POP_TO_ARG_0")
	emit("FUNCALL iruntime.malloc")

	emit("mov -24(%%rbp), %%rbx # newlen")
	emit("mov -16(%%rbp), %%rcx # newcap")

	emit("LEAVE_AND_RET")
	emitNewline()
}

func (f *DeclFunc) emit() {
	f.emitPrologue()
	f.body.emit()
	emit("mov $0, %%rax")
	emitFuncEpilogue(f.labelDeferHandler, f.stmtDefer)
}

func evalIntExpr(e Expr) int {
	switch e.(type) {
	case nil:
		errorf("e is nil")
	case *ExprNumberLiteral:
		return e.(*ExprNumberLiteral).val
	case *ExprVariable:
		errorft(e.token(), "variable cannot be inteppreted at compile time :%#v", e)
	case *Relation:
		return evalIntExpr(e.(*Relation).expr)
	case *ExprBinop:
		binop := e.(*ExprBinop)
		switch binop.op {
		case "+":
			return evalIntExpr(binop.left) + evalIntExpr(binop.right)
		case "-":
			return evalIntExpr(binop.left) - evalIntExpr(binop.right)
		case "*":
			return evalIntExpr(binop.left) * evalIntExpr(binop.right)

		}
	case *ExprConstVariable:
		cnst := e.(*ExprConstVariable)
		constVal, ok := cnst.val.(*Relation)
		if ok && constVal.name == "iota" {
			val, ok := constVal.expr.(*ExprConstVariable)
			if ok && val == eIota {
				return cnst.iotaIndex
			}
		}
		return evalIntExpr(cnst.val)
	default:
		errorft(e.token(), "unkown type %T", e)
	}
	return 0
}

