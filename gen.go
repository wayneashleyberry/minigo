package main

import (
	"fmt"
	"strings"
)

/**
  Intel® 64 and IA-32 Architectures Software Developer’s Manual
  Combined Volumes: 1, 2A, 2B, 2C, 2D, 3A, 3B, 3C, 3D and 4

  3.4.1.1 General-Purpose Registers in 64-Bit Mode

  In 64-bit mode, there are 16 general purpose registers and the default operand size is 32 bits.
  However, general-purpose registers are able to work with either 32-bit or 64-bit operands.
  If a 32-bit operand size is specified: EAX, EBX, ECX, EDX, EDI, ESI, EBP, ESP, R8D - R15D are available.
  If a 64-bit operand size is specified: RAX, RBX, RCX, RDX, RDI, RSI, RBP, RSP, R8-R15 are available.
  R8D-R15D/R8-R15 represent eight new general-purpose registers.
  All of these registers can be accessed at the byte, word, dword, and qword level.
  REX prefixes are used to generate 64-bit operand sizes or to reference registers R8-R15.
*/

var retRegi = [14]string{
	"rax", "rbx", "rcx", "rdx", "rdi", "rsi", "r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15",
}

var RegsForCall = [...]string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}

const IntSize = 8 // 64-bit (8 bytes)

var hiddenArrayId = 1

func getHidddenArrayId() int {
	r := hiddenArrayId
	hiddenArrayId++
	return r
}

func emit(format string, v ...interface{}) {
	fmt.Printf("\t"+format+"\n", v...)
}

func emitComment(format string, v ...interface{}) {
	fmt.Printf("/* "+format+" */\n", v...)
}

func emitLabel(format string, v ...interface{}) {
	fmt.Printf(format+"\n", v...)
}

// Mytype.method -> Mytype#method

func getMethodUniqueName(gtype *Gtype, fname identifier) string {
	assertNotNil(gtype != nil, nil)
	var typename identifier
	if gtype.typ == G_POINTER {
		typename = gtype.origType.relation.name
	} else {
		typename = gtype.relation.name
	}
	return string(typename) + "$" + string(fname)
}

// main.f1 -> main.f1
func getPackagedFuncName(pkg identifier, fname string) IrStaticCall {
	if pkg == "libc" {
		return IrStaticCall(fname)
	}

	return IrStaticCall(fmt.Sprintf("%s.%s", pkg, fname))
}

func (f *DeclFunc) getUniqueName() IrStaticCall {
	if f.receiver != nil {
		// method
		return getPackagedFuncName(f.pkg, getMethodUniqueName(f.receiver.gtype, f.fname))
	}

	// main.main => main
	if f.isMainMain {
		return "main"
	}

	// other functions
	return getPackagedFuncName(f.pkg, string(f.fname))
}

func (f *DeclFunc) emitPrologue() {
	if f.isMainMain {
		emit("# main.main")
	}
	uniquName := f.getUniqueName()
	//emitComment("func %s.%s()", f.pkg, f.fname)
	if f.getUniqueName() == "main" {
		emit(".global	%s", f.getUniqueName())
	}
	emitLabel("%s:", uniquName)
	emit("push %%rbp")
	emit("mov %%rsp, %%rbp")

	// calc offset
	var offset int
	var params []*ExprVariable
	if f.receiver != nil {
		params = []*ExprVariable{f.receiver}
		for _, param := range f.params {
			params = append(params, param)
		}
	} else {
		params = f.params
	}

	for i, param := range params {
		if i == 0 {
			emit("# Allocating stack for params")
		}
		offset -= IntSize
		param.offset = offset
		emit("push %%%s", RegsForCall[i])
	}

	var localarea int
	for i, lvar := range f.localvars {
		if i == 0 {
			emit("# Allocating stack for localvars")
		}
		if lvar.gtype == nil {
			debugf("%s has nil gtype ", lvar)
		}
		size := lvar.gtype.getSize()
		assert(size != 0, lvar.token(), "size should  not be zero:"+lvar.gtype.String())
		loff := align(size, 8)
		localarea -= loff
		offset -= loff
		lvar.offset = offset
		emit("# offset %d for variable \"%s\" of %s", offset, lvar.varname, lvar.gtype)
		//debugf("set offset %d to lvar %s, type=%s", lvar.offset, lvar.varname, lvar.gtype)
	}
	if localarea != 0 {
		emit("sub $%d, %%rsp # total stack size", -localarea)
	}

	emit("")
	if f.isMainMain {
		if importOS {
			emit("# set Args")
			emit("mov %%rsi, 0+Args(%%rip)")  // set pointer (**argv)
			emit("mov %%rdi, 8+Args(%%rip)")  // set len (argc)
			emit("mov %%rdi, 16+Args(%%rip)") // set cap (argc)
		}
	}
}

func align(n int, m int) int {
	remainder := n % m
	if remainder == 0 {
		return n
	} else {
		return n - remainder + m
	}
}

func emitFuncEpilogue() {
	emit("")
	emit("leave")
	emit("ret")
	emit("")
}

func (ast *ExprNumberLiteral) emit() {
	emit("mov	$%d, %%rax", ast.val)
}

func (ast *ExprStringLiteral) emit() {
	emit("lea .%s(%%rip), %%rax", ast.slabel)
}

func loadStructField(strct Expr, field *Gtype, offset int) {
	emit("# loadStructField")
	switch strct.(type) {
	case *Relation:
		rel := strct.(*Relation)
		assertNotNil(rel.expr != nil, nil)
		variable, ok := rel.expr.(*ExprVariable)
		assert(ok, nil, "rel is a variable")
		loadStructField(variable, field, offset)
	case *ExprVariable:
		variable := strct.(*ExprVariable)
		if field.typ == G_ARRAY {
			variable.emitAddress(field.offset)
		} else {
			if variable.isGlobal {
				emit("mov %s+%d(%%rip), %%rax # ", variable.varname, field.offset+offset)
			} else {
				emit("mov %d(%%rbp), %%rax", variable.offset+field.offset+offset)
			}
		}
	case *ExprStructField: // strct.field.field
		a := strct.(*ExprStructField)
		strcttype := a.strct.getGtype().relation.gtype
		assert(strcttype.size > 0, a.token(), "struct size should be > 0")
		field2 := strcttype.getField(a.fieldname)
		loadStructField(a.strct, field2, offset+field.offset)
	case *ExprIndex: // array[1].field
		indexExpr := strct.(*ExprIndex)
		loadCollectIndex(indexExpr.collection, indexExpr.index, offset+field.offset)
	default:
		// funcall().field
		// methodcall().field
		// *ptr.field
		// (MyStruct{}).field
		// (&MyStruct{}).field
		TBI(strct.token(), "unable to handle %T", strct)
	}

}

func (a *ExprStructField) emitAddress() {
	strcttype := a.strct.getGtype().origType.relation.gtype
	field := strcttype.getField(a.fieldname)
	a.strct.emit()
	emit("add $%d, %%rax", field.offset)
}

func (a *ExprStructField) emit() {
	emit("# ExprStructField.emit()")
	switch a.strct.getGtype().typ {
	case G_POINTER: // pointer to struct
		strcttype := a.strct.getGtype().origType.relation.gtype
		// very dirty hack
		if strcttype.size == undefinedSize {
			strcttype.calcStructOffset()
		}
		field := strcttype.getField(a.fieldname)
		a.strct.emit()
		emit("add $%d, %%rax", field.offset)
		emit("mov (%%rax), %%rax")
	case G_REL: // struct
		strcttype := a.strct.getGtype().relation.gtype
		assert(strcttype.size > 0, a.token(), "struct size should be > 0")
		field := strcttype.getField(a.fieldname)
		loadStructField(a.strct, field, 0)
	default:
		errorft(a.token(), "internal error: bad gtype %s", a.strct.getGtype())
	}
}

func (ast *ExprVariable) emit() {
	emit("# emit variable")
	if ast.gtype.typ == G_ARRAY {
		ast.emitAddress(0)
		return
	} else if ast.gtype.typ == G_REL && ast.gtype.relation.gtype.typ == G_INTERFACE {
		if ast.isGlobal {
			emit("mov %s+%d(%%rip), %%rcx", ast.varname, ptrSize + ptrSize)
			emit("mov %s+%d(%%rip), %%rbx", ast.varname, ptrSize)
			emit("mov %s(%%rip), %%rax", ast.varname)
		} else {
			emit("mov %d(%%rbp), %%rcx", ast.offset + ptrSize + ptrSize)
			emit("mov %d(%%rbp), %%rbx", ast.offset + ptrSize)
			emit("mov %d(%%rbp), %%rax", ast.offset)
		}
		return
	}

	if ast.isGlobal {
		emit("mov %s(%%rip), %%rax", ast.varname)
	} else {
		if ast.offset == 0 {
			errorft(ast.token(), "offset should not be zero for localvar %s", ast.varname)
		}
		switch {
		case ast.getGtype().typ == G_SLICE:
			emit("#   emit slice variable")
			emit("mov %d(%%rbp), %%rax", ast.offset)
			emit("mov %d(%%rbp), %%rbx", ast.offset+ptrSize)
			emit("mov %d(%%rbp), %%rcx", ast.offset+ptrSize+IntSize)
		default:
			emit("mov %d(%%rbp), %%rax", ast.offset)
		}
	}
}

func (variable *ExprVariable) emitAddress(offset int) {
	if variable.isGlobal {
		emit("lea %s+%d(%%rip), %%rax", variable.varname, offset)
	} else {
		if variable.offset == 0 {
			errorft(variable.token(), "offset should not be zero for localvar %s", variable.varname)
		}
		emit("lea %d(%%rbp), %%rax", variable.offset+offset)
	}
}

func (rel *Relation) emit() {
	if rel.expr == nil {
		errorft(rel.token(), "rel.expr is nil: %s", rel.name)
	}
	rel.expr.emit()
}

func (ast *ExprConstVariable) emit() {
	assertNotNil(ast.val != nil, nil)
	rel, ok := ast.val.(*Relation)
	if ok && rel.expr == eIota {
		// replace the iota expr by a index number
		val := &ExprNumberLiteral{
			val: ast.iotaIndex,
		}
		val.emit()
	} else {
		ast.val.emit()
	}
}

func emit_comp_primitive(inst string, binop *ExprBinop) {
	binop.left.emit()
	emit("push %%rax")
	binop.right.emit()
	emit("pop %%rcx")
	emit("cmp %%rax, %%rcx") // right, left
	emit("%s %%al", inst)
	emit("movzb %%al, %%eax")
}

var labelSeq = 0

func makeLabel() string {
	r := fmt.Sprintf(".L%d", labelSeq)
	labelSeq++
	return r
}

func (ast *StmtInc) emit() {
	emitIncrDecl("add", ast.operand)
}
func (ast *StmtDec) emit() {
	emitIncrDecl("sub", ast.operand)
}

// https://golang.org/ref/spec#IncDecStmt
// As with an assignment, the operand must be addressable or a map index expression.
func emitIncrDecl(inst string, operand Expr) {
	operand.emit()
	emit("%s $1, %%rax", inst)

	left := operand
	emitSave(left)
}

// e.g. *x = 1, or *x++
func (uop *ExprUop) emitSave() {
	emit("# *ExprUop.emitSave()")
	assert(uop.op == "*", uop.tok, "uop op should be *")
	emit("push %%rax")
	uop.operand.emit()
	emit("mov %%rax, %%rcx")
	emit("pop %%rax")
	reg := getReg(uop.operand.getGtype().getSize())
	emit("mov %%%s, (%%rcx)", reg)

}

// e.g. x = 1
func (rel *Relation) emitSave() {
	if rel.expr == nil {
		errorft(rel.token(), "left.rel.expr is nil")
	}
	variable := rel.expr.(*ExprVariable)
	variable.emitOffsetSave(variable.getGtype().getSize(), 0)
}

func (variable *ExprVariable) emitOffsetSave(size int, offset int) {
	emit("# ExprVariable.emitOffsetSave")
	assert(0 <= size && size <= 8, variable.token(), "invalid size")
	if variable.getGtype().typ == G_POINTER && offset > 0 {
		assert(variable.getGtype().typ == G_POINTER, variable.token(), "")
		emit("push %%rax # save the value")
		variable.emit()
		emit("mov %%rax, %%rcx")
		emit("add $%d, %%rcx", offset)
		emit("pop %%rax")
		emit("mov %%rax, (%%rcx)")
		return
	}
	if variable.isGlobal {
		emitGsave(size, variable.varname, offset)
	} else {
		emitLsave(size, variable.offset+offset)
	}
}

func (variable *ExprVariable) emitOffsetLoad(size int, offset int) {
	assert(0 <= size && size <= 8, variable.token(), "invalid size")
	if variable.isGlobal {
		emitGload(size, variable.varname, offset)
	} else {
		emitLload(size, variable.offset+offset)
	}
}

func (ast *ExprUop) emit() {
	//debugf("emitting ExprUop")
	if ast.op == "&" {
		switch ast.operand.(type) {
		case *Relation:
			rel := ast.operand.(*Relation)
			vr, ok := rel.expr.(*ExprVariable)
			if !ok {
				errorft(ast.token(), "rel is not an variable")
			}
			vr.emitAddress(0)
		case *ExprStructLiteral:
			e := ast.operand.(*ExprStructLiteral)
			assert(e.invisiblevar.offset != 0, nil, "ExprStructLiteral's invisible var has offset")
			assignToStruct(e.invisiblevar, e)
			// @TODO handle global vars
			emit("lea %d(%%rbp), %%rax", e.invisiblevar.offset)
		case *ExprStructField:
			e := ast.operand.(*ExprStructField)
			e.emitAddress()
		default:
			errorft(ast.token(), "Unknown type: %T", ast.operand)
		}
	} else if ast.op == "*" {
		// dereferene of a pointer
		//debugf("dereferene of a pointer")
		//rel, ok := ast.operand.(*Relation)
		//debugf("operand:%s", rel)
		//vr, ok := rel.expr.(*ExprVariable)
		//assert(ok, nil, "operand is a rel")
		ast.operand.emit()
		emit("mov (%%rax), %%rcx")
		emit("mov %%rcx, %%rax")
	} else if ast.op == "!" {
		ast.operand.emit()
		emit("mov $0, %%rcx")
		emit("cmp %%rax, %%rcx")
		emit("sete %%al")
		emit("movzb %%al, %%eax")
	} else if ast.op == "-" {
		// delegate to biop
		// -(x) -> (-1) * (x)
		left := &ExprNumberLiteral{val: -1}
		binop := &ExprBinop{
			op:    "*",
			left:  left,
			right: ast.operand,
		}
		binop.emit()
	} else {
		errorft(ast.token(), "unable to handle uop %s", ast.op)
	}
	//debugf("end of emitting ExprUop")

}

func (binop *ExprBinop) emitCompareStrings() {
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
		TBI(binop.token(), "")
	case "==":
		binop.left.emit()
		emit("push %%rax")
		binop.right.emit()
		emit("pop %%rcx")
		emit("# rax = right, rcx = left")
		emitStringsEqual("%rcx", "%rax")

	}

}

// call strcmp
func emitStringsEqual(leftReg string, rightReg string) {
	emit("mov %s, %%rsi", leftReg)
	emit("mov %s, %%rdi", rightReg)
	emit("mov $0, %%rax")
	emit("call strcmp")
	emit("cmp $0, %%rax") // retval == 0
	emit("sete %%al")
	emit("movzb %%al, %%eax")
}

func (binop *ExprBinop) emitComp() {
	switch {
	case binop.left.getGtype().typ == G_STRING ||
		(binop.left.getGtype().typ == G_REL && binop.left.getGtype().relation.gtype.typ == G_STRING):
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

func (ast *ExprBinop) emit() {
	switch ast.op {
	case "<", ">", "<=", ">=", "!=", "==":
		ast.emitComp()
		return
	case "&&":
		labelEnd := makeLabel()
		ast.left.emit()
		emit("test %%rax, %%rax")
		emit("mov $0, %%rax")
		emit("je %s", labelEnd)
		ast.right.emit()
		emit("test %%rax, %%rax")
		emit("mov $0, %%rax")
		emit("je %s", labelEnd)
		emit("mov $1, %%rax")
		emit("%s:", labelEnd)
		return
	case "||":
		labelEnd := makeLabel()
		ast.left.emit()
		emit("test %%rax, %%rax")
		emit("mov $1, %%rax")
		emit("jne %s", labelEnd)
		ast.right.emit()
		emit("test %%rax, %%rax")
		emit("mov $1, %%rax")
		emit("jne %s", labelEnd)
		emit("mov $0, %%rax")
		emit("%s:", labelEnd)
		return
	}
	ast.left.emit()
	emit("push %%rax")
	ast.right.emit()
	emit("mov %%rax, %%rcx")
	emit("pop %%rax")
	if ast.op == "+" {
		emit("add	%%rcx, %%rax")
	} else if ast.op == "-" {
		emit("sub	%%rcx, %%rax")
	} else if ast.op == "*" {
		emit("imul	%%rcx, %%rax")
	} else if ast.op == "%" {
		emit("mov $0, %%rdx # init %%rdx")
		emit("div %%rcx")
		emit("mov %%rdx, %%rax")
	} else if ast.op == "/" {
		emit("mov $0, %%rdx # init %%rdx")
		emit("mov $0, %%rdx")
		emit("div %%rcx")
	} else {
		errorft(ast.token(), "Unknown binop: %s", ast.op)
	}
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
	emit("")
	emit("# Assignment")
	// the right hand operand is a single multi-valued expression
	// such as a function call, a channel or map operation, or a type assertion.
	// The number of operands on the left hand side must match the number of values.
	isOnetoOneAssignment := (len(ast.rights) > 1)
	if isOnetoOneAssignment {
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
			case gtype.typ == G_ARRAY:
				assignToArray(left, right)
			case gtype.typ == G_SLICE:
				assignToSlice(left, right)
			case gtype.typ == G_REL && gtype.relation.gtype.typ == G_STRUCT:
				assignToStruct(left, right)
			case gtype.typ == G_REL && gtype.relation.gtype.typ == G_INTERFACE:
				assignToInterface(left, right)
			default:
				// suppose primitive
				emitAssignPrimitive(left, right)
			}
		}
		return
	} else {
		// a,b,c = expr
		numLeft := len(ast.lefts)
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
		default:
			numRight++
		}

		if leftsMayBeTwo {
			if numLeft > 2 {
				errorft(ast.token(), "number of exprs does not match")
			}
		} else {
			if numLeft != numRight {
				errorft(ast.token(), "number of exprs does not match")
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
					if left.getGtype().typ == G_SLICE {
						// @TODO: Does this work ?
						emitSaveSlice(left, 0)
					} else if left.getGtype().typ == G_REL && left.getGtype().relation.gtype.typ == G_INTERFACE {
						// @TODO: Does this work ?
						emitSaveInterface(left, 0)
					} else {
						emit("pop %%rax")
						emitSave(left)
					}
				}
				return
			}
		}

		gtype := left.getGtype()
		switch {
		case gtype.typ == G_ARRAY:
			assignToArray(left, right)
		case gtype.typ == G_SLICE:
			assignToSlice(left, right)
		case gtype.typ == G_REL && gtype.relation.gtype.typ == G_STRUCT:
			assignToStruct(left, right)
		case gtype.typ == G_REL && gtype.relation.gtype.typ == G_INTERFACE:
			assignToInterface(left, right)
		default:
			// suppose primitive
			emitAssignPrimitive(left, right)
		}
		if leftsMayBeTwo && len(ast.lefts) == 2 {
			okVariable := ast.lefts[1]
			// @TODO consider big data like slice, struct, etd
			emit("mov %%rbx, %%rax") // ok
			emitSave(okVariable)
		}
		return
	}

}

func emitAssignPrimitive(left Expr, right Expr) {
	right.emit()   //   expr => %rax
	emitSave(left) //   %rax => memory
}

// Each left-hand side operand must be addressable,
// a map index expression,
// or (for = assignments only) the blank identifier.
func emitSave(left Expr) {
	switch left.(type) {
	case *Relation:
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

// m[k] = v
// append key and value to the tail of map data, and increment its length
func (e *ExprIndex) emitMapSet() {

	labelAppend := makeLabel()
	labelSave := makeLabel()

	// map get to check if exists
	e.emit()
	// jusdge update or append
	emit("cmp $0, %%rcx")
	emit("setne %%al")
	emit("movzb %%al, %%eax")
	emit("test %%rax, %%rax")
	emit("je %s  # jump to append if not found", labelAppend)

	// update
	emit("push %%rcx") // push address of the key
	emit("jmp %s", labelSave)

	// append
	emit("%s: # append to a map ", labelAppend)
	e.collection.emit() // emit pointer address to %rax
	emit("push %%rax # stash head address of mapData")

	// emit len of the map
	elen := &ExprLen{
		arg: e.collection,
	}
	elen.emit()
	emit("imul $%d, %%rax", 2*8) // distance from head to tail
	emit("pop %%rcx")            // head
	emit("add %%rax, %%rcx")     // now rcx is the tail address
	emit("push %%rcx")

	// map len++
	elen.emit()
	emit("add $1, %%rax")
	emitOffsetSave(e.collection, IntSize, ptrSize) // update map len

	// Save key and value
	emit("%s: # end loop", labelSave)
	e.index.emit()
	emit("push %%rax") // index value

	mapType := e.collection.getGtype()
	mapKeyType := mapType.mapKey
	mapValueType := mapType.mapValue

	if mapKeyType.isString() {
		emit("pop %%rcx")            // index value
		emit("pop %%rax")            // map tail address
		emit("mov %%rcx, (%%rax)")   // save indexvalue to malloced area
		emit("push %%rax")           // push map tail
	} else {
		// malloc(8)
		emit("mov $%d, %%rdi", 8) // malloc 8 bytes
		emit("mov $0, %%rax")
		emit("call .malloc")
		// %%rax : malloced address
		// stack : [map tail address, index value]
		emit("pop %%rcx")            // index value
		emit("mov %%rcx, (%%rax)")   // save indexvalue to malloced area
		emit("pop %%rcx")            // map tail address
		emit("mov %%rax, (%%rcx) #") // save index address to the tail
		emit("push %%rcx")           // push map tail
	}

	// save value
	if mapValueType.isString() {
		emit("pop %%rax")           // map tail address
		emit("pop %%rcx") // rhs value
		// save value
		emit("mov %%rcx, 8(%%rax)") // save value to the tail
	} else {
		// malloc(8)
		emit("mov $%d, %%rdi", 8) // malloc 8 bytes
		emit("mov $0, %%rax")
		emit("call .malloc")

		emit("pop %%rcx")           // map tail address
		emit("mov %%rax, 8(%%rcx)") // set malloced address to tail+8

		emit("pop %%rcx") // rhs value

		// save value
		emit("mov %%rcx, (%%rax)") // save value address to the malloced area
	}
}

func (e *ExprIndex) emitSave() {
	emit("push %%rax") // push RHS value

	// load head address of the array
	// load index
	// multi index * size
	// calc address = head address + offset
	// copy value to the address

	collectionType := e.collection.getGtype()
	switch {
	case collectionType.typ == G_ARRAY, collectionType.typ == G_SLICE:
		e.collection.emit() // head address
	case collectionType.typ == G_MAP:
		e.emitMapSet()
		return
	default:
		TBI(e.token(), "unable to handle %s", collectionType)
	}

	emit("push %%rax # stash head address of collection")
	e.index.emit()
	emit("mov %%rax, %%rcx") // index
	elmType := collectionType.elementType
	size := elmType.getSize()
	assert(size > 0, nil, "size > 0")
	emit("mov $%d, %%rax # size of one element", size)
	emit("imul %%rcx, %%rax # index * size")
	emit("push %%rax # store index * size")
	emit("pop %%rcx # oad  index * size")
	emit("pop %%rbx # load address of variable")
	emit("add %%rcx , %%rbx # (index * size) + address")
	emit("pop %%rax # load RHS value")
	reg := getReg(size)
	emit("mov %%%s, (%%rbx) # finally save value to an element", reg)
}

func (e *ExprStructField) emitSave() {
	fieldType := e.getGtype()
	emitOffsetSave(e.strct, 8, fieldType.offset)
}

func (e *ExprStructField) emitOffsetLoad(size int, offset int) {
	rel, ok := e.strct.(*Relation)
	assert(ok, e.tok, "should be *Relation")
	vr, ok := rel.expr.(*ExprVariable)
	assert(ok, e.tok, "should be *ExprVariable")
	assert(vr.gtype.typ == G_REL, e.tok, "expect G_REL, but got "+vr.gtype.String())
	field := vr.gtype.relation.gtype.getField(e.fieldname)
	vr.emitOffsetLoad(size, field.offset+offset)
}

func (stmt *StmtIf) emit() {
	emit("# if")
	if stmt.simplestmt != nil {
		stmt.simplestmt.emit()
	}
	stmt.cond.emit()
	emit("test %%rax, %%rax")
	if stmt.els != nil {
		labelElse := makeLabel()
		labelEndif := makeLabel()
		emit("je %stmt  # jump if 0", labelElse)
		emit("# then block")
		stmt.then.emit()
		emit("jmp %stmt # jump to endif", labelEndif)
		emit("# else block")
		emit("%stmt:", labelElse)
		stmt.els.emit()
		emit("# endif")
		emit("%stmt:", labelEndif)
	} else {
		// no else block
		labelEndif := makeLabel()
		emit("je %stmt  # jump if 0", labelEndif)
		emit("# then block")
		stmt.then.emit()
		emit("# endif")
		emit("%stmt:", labelEndif)
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
		emit("push %%rax")
		emit("#")
	}

	// case exp1,exp2,..:
	//     stmt1;
	//     stmt2;
	//     ...
	for i, caseClause := range stmt.cases {
		emit("# case %d", i)
		myCaseLabel := makeLabel()
		labels = append(labels, myCaseLabel)
		if stmt.isTypeSwitch {
			// compare type
			for _, gtype := range caseClause.gtypes {
				typeLabel := groot.getTypeLabel(gtype)
				emit("lea .%s(%%rip), %%rax # type: %s", typeLabel, gtype)
				emit("pop %%rcx # the subject type")
				emitStringsEqual("%rax", "%rcx")
				emit("test %%rax, %%rax")
				emit("jne %s # jump if matches", myCaseLabel)
				emit("push %%rcx # the subject value")
			}
		} else {
			for _, e := range caseClause.exprs {
				e.emit()
				emit("pop %%rcx # the subject value")
				emit("cmp %%rax, %%rcx") // right, left
				emit("sete %%al")
				emit("movzb %%al, %%eax")
				emit("test %%rax, %%rax")
				emit("jne %s # jump if matches", myCaseLabel)
				emit("push %%rcx # the subject value")
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

	emit("pop %%rax # destroy the subject value")
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

func (f *StmtFor) emitMapRange() {

	labelBegin := makeLabel()
	labelEnd := makeLabel()

	mapCounter := &Relation{
		name: "",
		expr: f.rng.invisibleMapCounter,
	}
	// counter = 0
	initstmt := &StmtAssignment{
		lefts: []Expr{
			mapCounter,
		},
		rights: []Expr{
			&ExprNumberLiteral{
				val: 0,
			},
		},
	}
	emit("# init index")
	initstmt.emit()

	emit("%s: # begin loop ", labelBegin)

	// counter < len(list)
	condition := &ExprBinop{
		op:   "<",
		left: mapCounter, // i
		// @TODO
		// The range expression x is evaluated once before beginning the loop
		right: &ExprLen{
			arg: f.rng.rangeexpr, // len(expr)
		},
	}
	condition.emit()
	emit("test %%rax, %%rax")
	emit("je %s  # jump if false", labelEnd)

	// set key and value
	mapCounter.emit()
	emit("imul $16, %%rax")
	emit("push %%rax")
	f.rng.rangeexpr.emit() // emit address of map data head
	mapType := f.rng.rangeexpr.getGtype()
	mapKeyType := mapType.mapKey
	mapValueType := mapType.mapValue
	emit("pop %%rcx")
	emit("add %%rax, %%rcx")
	emit("mov (%%rcx), %%rax")
	if !mapKeyType.isString() {
		emit("mov (%%rax), %%rax")
	}
	f.rng.indexvar.emitSave()

	if f.rng.valuevar != nil {
		emit("# Setting valuevar")
		emit("## rangeexpr.emit()")
		f.rng.rangeexpr.emit()
		emit("mov %%rax, %%rcx")

		emit("## mapCounter.emit()")
		mapCounter.emit()

		emit("## eval value")
		emit("imul $16, %%rax")
		emit("add $8, %%rax")
		emit("add %%rax, %%rcx")
		emit("mov (%%rcx), %%rax")
		if !mapValueType.isString() {
			emit("mov (%%rax), %%rax")
		}
		f.rng.valuevar.emitSave()

	}

	// execute body
	f.block.emit()

	// counter++
	indexIncr := &StmtInc{
		operand: mapCounter,
	}
	indexIncr.emit()

	emit("jmp %s", labelBegin)
	emit("%s: # end loop", labelEnd)
}

func (f *StmtFor) emitRange() {
	emit("# for range %T", f.rng.rangeexpr.getGtype())
	assertNotNil(f.rng.indexvar != nil, f.rng.tok)
	if f.rng.rangeexpr.getGtype().typ == G_MAP {
		f.emitMapRange()
		return
	}
	assert(f.rng.rangeexpr.getGtype().typ == G_ARRAY || f.rng.rangeexpr.getGtype().typ == G_SLICE, f.rng.tok, "rangeexpr should be G_ARRAY or G_SLICE")

	labelBegin := makeLabel()
	labelEnd := makeLabel()

	// i = 0
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
	emit("# init index")
	initstmt.emit()

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
	emit("test %%rax, %%rax")
	emit("je %s  # jump if false", labelEnd)

	f.block.emit()

	// i++
	indexIncr := &StmtInc{
		operand: f.rng.indexvar,
	}
	indexIncr.emit()

	// v = s[i]
	if f.rng.valuevar != nil {
		assignVar.emit()
	}
	emit("jmp %s", labelBegin)
	emit("%s: # end loop", labelEnd)
}

func (f *StmtFor) emitForClause() {
	assertNotNil(f.cls != nil, nil)
	labelBegin := makeLabel()
	labelEnd := makeLabel()

	if f.cls.init != nil {
		f.cls.init.emit()
	}
	emit("%s: # begin loop ", labelBegin)
	if f.cls.cond != nil {
		f.cls.cond.emit()
		emit("test %%rax, %%rax")
		emit("je %s  # jump if false", labelEnd)
	}
	f.block.emit()
	if f.cls.post != nil {
		f.cls.post.emit()
	}
	emit("jmp %s", labelBegin)
	emit("%s: # end loop", labelEnd)
}

func (f *StmtFor) emit() {
	if f.rng != nil {
		f.emitRange()
		return
	}
	f.emitForClause()
}

func (stmt *StmtReturn) emit() {
	if len(stmt.exprs) == 0 {
		emit("mov $0, %%rax")
		emit("leave")
		emit("ret")
		return
	}

	if len(stmt.exprs) > 7 {
		TBI(stmt.token(), "too many number of arguments")
	}

	var retRegiIndex int
	if len(stmt.exprs) == 1 {
		expr := stmt.exprs[0]
		expr.emit()
		if expr.getGtype() == nil && stmt.rettypes[0].typ == G_SLICE {
			emit("mov $0, %%rbx")
			emit("mov $0, %%rcx")
		}
		emit("leave")
		emit("ret")
		return
	}
	for i, rettype := range stmt.rettypes {
		expr := stmt.exprs[i]
		expr.emit()
		//		rettype := stmt.rettypes[i]
		if expr.getGtype() == nil && rettype.typ == G_SLICE {
			emit("mov $0, %%rbx")
			emit("mov $0, %%rcx")
		}
		size := rettype.getSize()
		if size < 8 {
			size = 8
		}
		var num64bit int = size / 8 // @TODO odd size
		for j := 0; j < num64bit; j++ {
			emit("push %%%s", retRegi[num64bit-1-j])
			retRegiIndex++
		}
	}
	for i := 0; i < retRegiIndex; i++ {
		emit("pop %%%s", retRegi[retRegiIndex-1-i])
	}

	emit("leave")
	emit("ret")
}

func getReg(regSize int) string {
	var reg string
	switch regSize {
	case 1:
		reg = "al"
	case 8:
		reg = "rax"
	default:
		errorf("Unexpected reg size %d", regSize)

	}
	return reg
}

func emitLsave(regSize int, loff int) {
	reg := getReg(regSize)
	emit("mov %%%s, %d(%%rbp)", reg, loff)
}

func emitGsave(regSize int, varname identifier, offset int) {
	reg := getReg(regSize)
	emit("mov %%%s, %s+%d(%%rip)", reg, varname, offset)
}

func emitLload(regSize int, loff int) {
	reg := getReg(regSize)
	emit("mov %d(%%rbp), %%%s", loff, reg)
}

func emitGload(regSize int, varname identifier, offset int) {
	reg := getReg(regSize)
	emit("mov %s+%d(%%rip), %%%s", varname, offset, reg)
}

func assignToStruct(lhs Expr, rhs Expr) {
	emit("# assignToStruct")
	if rel, ok := lhs.(*Relation); ok {
		lhs = rel.expr
	}
	assert(rhs == nil || (rhs.getGtype().typ == G_REL && rhs.getGtype().relation.gtype.typ == G_STRUCT),
		lhs.token(), "rhs should be struct type")
	// initializes with zero values
	for _, fieldtype := range lhs.getGtype().relation.gtype.fields {
		//debugf("%#v", fieldtype)
		switch {
		case fieldtype.typ == G_ARRAY:
			arrayType := fieldtype
			elementType := arrayType.elementType
			elmSize := arrayType.elementType.getSize()
			switch {
			case elementType.typ == G_REL && elementType.relation.gtype.typ == G_STRUCT:
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

		case fieldtype.typ == G_SLICE:
			emit("# initialize slice with a zero value")
			emit("push $0")
			emit("push $0")
			emit("push $0")
			emitSaveSlice(lhs, fieldtype.offset)
		case fieldtype.typ == G_REL && fieldtype.relation.gtype.typ == G_STRUCT:
			left := &ExprStructField{
				strct:     lhs,
				fieldname: fieldtype.fieldname,
			}
			assignToStruct(left, nil)
		case fieldtype.getPrimType() == G_INTERFACE:
			emit("push $0")
			emit("push $0")
			emit("push $0")
			emitSaveInterface(lhs, fieldtype.offset)
		default:
			emit("mov $0, %%rax")
			regSize := fieldtype.getSize()
			assert(0 < regSize && regSize <= 8, lhs.token(), fieldtype.String())
			emitOffsetSave(lhs, regSize, fieldtype.offset)
		}
	}

	if rhs == nil {
		return
	}
	variable := lhs

	structliteral, ok := rhs.(*ExprStructLiteral)
	assert(ok || rhs == nil, nil, fmt.Sprintf("invalid rhs: %T", rhs))
	strcttyp := structliteral.strctname.gtype

	// do assignment for each field
	for _, field := range structliteral.fields {
		emit("# .%s", field.key)
		fieldtype := strcttyp.getField(field.key)

		switch {
		case fieldtype.typ == G_ARRAY:
			initvalues, ok := field.value.(*ExprArrayLiteral)
			assert(ok, nil, "ok")
			fieldtype := strcttyp.getField(field.key)
			arrayType := fieldtype
			elementType := arrayType.elementType
			elmSize := elementType.getSize()
			switch {
			case elementType.typ == G_REL && elementType.relation.gtype.typ == G_STRUCT:
				left := &ExprStructField{
					strct:     lhs,
					fieldname: fieldtype.fieldname,
				}
				assignToArray(left, field.value)
			default:
				for i, val := range initvalues.values {
					val.emit()
					emitOffsetSave(variable, elmSize, fieldtype.offset+i*elmSize)
				}
			}
		case fieldtype.typ == G_SLICE:
			left := &ExprStructField{
				tok:       variable.token(),
				strct:     lhs,
				fieldname: field.key,
			}
			assignToSlice(left, field.value)
		case fieldtype.getPrimType() == G_INTERFACE:
			left := &ExprStructField{
				tok:       lhs.token(),
				strct:     lhs,
				fieldname: field.key,
			}
			assignToInterface(left, field.value)
		case fieldtype.typ == G_REL && fieldtype.relation.gtype.typ == G_STRUCT:
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

}

const sliceOffsetForLen = 8

func emitOffsetSave(lhs Expr, size int, offset int) {
	switch lhs.(type) {
	case *Relation:
		rel := lhs.(*Relation)
		emitOffsetSave(rel.expr, size, offset)
	case *ExprVariable:
		variable := lhs.(*ExprVariable)
		variable.emitOffsetSave(size, offset)
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

func emitOffsetLoad(lhs Expr, size int, offset int) {
	switch lhs.(type) {
	case *Relation:
		rel := lhs.(*Relation)
		emitOffsetLoad(rel.expr, size, offset)
	case *ExprVariable:
		variable := lhs.(*ExprVariable)
		variable.emitOffsetLoad(size, offset)
	case *ExprStructField:
		structfield := lhs.(*ExprStructField)
		fieldType := structfield.getGtype()
		emitOffsetLoad(structfield.strct, size, fieldType.offset+offset)
	case *ExprIndex:
		TBI(lhs.token(), "Unable to assign to %T", lhs)
	default:
		errorft(lhs.token(), "unkonwn type %T", lhs)
	}
}

func emitSaveInterface(lhs Expr, offset int) {
	switch lhs.(type) {
	case *Relation:
		rel := lhs.(*Relation)
		emitSaveInterface(rel.expr, offset)
	case *ExprVariable:
		variable := lhs.(*ExprVariable)
		variable.saveInterface(offset)
	case *ExprStructField:
		structfield := lhs.(*ExprStructField)
		fieldType := structfield.getGtype()
		emitSaveInterface(structfield.strct, fieldType.offset+offset)
	case *ExprIndex:
		TBI(lhs.token(), "Unable to assign to %T", lhs)
	default:
		errorft(lhs.token(), "unkonwn type %T", lhs)
	}
}

// take slice values from stack
func emitSaveSlice(lhs Expr, offset int) {
	switch lhs.(type) {
	case *Relation:
		rel := lhs.(*Relation)
		emitSaveSlice(rel.expr, offset)
	case *ExprVariable:
		variable := lhs.(*ExprVariable)
		variable.saveSlice(offset)
	case *ExprStructField:
		structfield := lhs.(*ExprStructField)
		fieldType := structfield.getGtype()
		emitSaveSlice(structfield.strct, fieldType.offset+offset)
	case *ExprIndex:
		TBI(lhs.token(), "Unable to assign to %T", lhs)
	default:
		errorft(lhs.token(), "unkonwn type %T", lhs)
	}
}

func emitCallMalloc(size int) {
	emit("mov $%d, %%rdi", size)
	emit("mov $0, %%rax")
	emit("call .malloc")
	// @TODO check malloc error
}

func assignToMap(lhs Expr, rhs Expr) {
	emit("# assignToMap")
	if rhs == nil {
		emit("# initialize slice with a zero value")
		emit("push $0")
		emit("push $0")
		emit("push $0")
		emitSaveSlice(lhs, 0)
		return
	}
	switch rhs.(type) {
	case *ExprMapLiteral:
		emit("# map literal")

		lit := rhs.(*ExprMapLiteral)
		length := len(lit.elements)

		// allocaated address of the map head
		emitCallMalloc(length*8*4)
		emit("push %%rax")

		mapType := rhs.getGtype()
		mapKeyType := mapType.mapKey
		mapValueType := mapType.mapValue

		for i, element := range lit.elements {
			// alloc key
			element.key.emit()
			emit("push %%rax") // value of key

			if mapKeyType.isString() {
				emit("pop %%rcx")          // value of key
				emit("pop %%r10")                     // map head
				emit("mov %%rcx, %d(%%r10) #", i*2*8) // save key address
				emit("push %%r10")
			} else {
				// call malloc for key
				emitCallMalloc(8)
				emit("pop %%rcx")          // value of key
				emit("mov %%rcx, (%%rax)") // save key to heap

				emit("pop %%r10")                     // map head
				emit("mov %%rax, %d(%%r10) #", i*2*8) // save key address
				emit("push %%r10")
			}

			element.value.emit()
			emit("push %%rax") // value of value

			if mapValueType.isString() {
				emit("pop %%rcx")          // value of value
				emit("pop %%r10") // map head
				emit("mov %%rcx, %d(%%r10) #", i*2*8+8)
				emit("push %%r10")
			} else {
				// call malloc
				emitCallMalloc(8)
				emit("pop %%rcx")          // value of value
				emit("mov %%rcx, (%%rax)") // save value to heap

				emit("pop %%r10") // map head
				emit("mov %%rax, %d(%%r10) #", i*2*8+8)
				emit("push %%r10")
			}

		}

		emit("pop %%rax")
		emit("push %%rax")       // address (head of the heap)
		emit("push $%d", length) // len
		emit("push $%d", length) // cap
	default:
		TBI(rhs.token(), "unable to handle %T", rhs)
	}
	emitSaveSlice(lhs, 0)
}

func convertDynamicTypeToInterface(dynamicValue Expr) {
	dynamicValue.emit()
	emit("push %%rax")
	emitCallMalloc(8)
	emit("pop %%rcx") // dynamicValue
	emit("mov %%rcx, (%%rax)") // store value to heap
	emit("push %%rax")  // address

	namedType := dynamicValue.getGtype()
	if namedType.typ == G_POINTER {
		namedType = namedType.origType.relation.gtype
	}
	assert(namedType.typeId > 0,  dynamicValue.token(), "no typeId")
	emit("mov $%d, %%rax # typeId", namedType.typeId)

	emit("push %%rax") // namedType id

	gtype := dynamicValue.getGtype()
	dtypeId := groot.hashedTypes[gtype.String()]
	label := fmt.Sprintf("DT%d", dtypeId)
	emit("lea .%s, %%rax# dtype %s",label,  gtype.String())
	emit("push %%rax")

}

func assignToInterface(lhs Expr, rhs Expr) {
	emit("# assignToInterface")
	if rhs == nil {
		emit("# initialize interface with a zero value")
		emit("push $0")
		emit("push $0")
		emit("push $0")
		emitSaveInterface(lhs, 0)
		return
	}

	if rhs.getGtype().typ == G_REL && rhs.getGtype().relation.gtype.typ == G_INTERFACE {
		rhs.emit()
		emit("push %%rax")
		emit("push %%rbx")
		emit("push %%rcx")
		emitSaveInterface(lhs, 0)
		return
	}

	convertDynamicTypeToInterface(rhs)
	emitSaveInterface(lhs, 0)
}

func assignToSlice(lhs Expr, rhs Expr) {
	emit("# assignToSlice")
	//assert(rhs == nil || rhs.getGtype().typ == G_SLICE, nil, "should be a slice literal or nil")
	if rhs == nil {
		emit("# initialize slice with a zero value")
		emit("push $0")
		emit("push $0")
		emit("push $0")
		emitSaveSlice(lhs, 0)
		return
	}

	switch rhs.(type) {
	case *Relation:
		rel := rhs.(*Relation)
		if _, ok := rel.expr.(*ExprNilLiteral); ok {
			// already initialied above
			return
		}
		rvariable, ok := rel.expr.(*ExprVariable)
		assert(ok, nil, "ok")
		// copy address
		rvariable.emit()
		emit("push %%rax")

		// copy len
		emit("mov %d(%%rbp), %%rax", rvariable.offset+ptrSize)
		emit("push %%rax")

		// copy cap
		emit("mov %d(%%rbp), %%rax", rvariable.offset+ptrSize+sliceOffsetForLen)
		emit("push %%rax")
	case *ExprSliceLiteral:
		lit := rhs.(*ExprSliceLiteral)
		// initialize a hidden array
		arrayLiteral := &ExprArrayLiteral{
			gtype:  lit.invisiblevar.gtype,
			values: lit.values,
		}
		assignToArray(lit.invisiblevar, arrayLiteral)
		lit.invisiblevar.emitAddress(0)
		emit("push %%rax")
		emit("push $%d", lit.invisiblevar.gtype.length) // len
		emit("push $%d", lit.invisiblevar.gtype.length) // cap
	case *ExprSlice:
		e := rhs.(*ExprSlice)
		e.emitToStack()
	case *ExprConversion:
		// https://golang.org/ref/spec#Conversions
		// Converting a value of a string type to a slice of bytes type
		// yields a slice whose successive elements are the bytes of the string.
		//
		// see also https://blog.golang.org/strings
		conversion := rhs.(*ExprConversion)
		assert(conversion.gtype.typ == G_SLICE, rhs.token(), "must be a slice of bytes")
		assert(conversion.expr.getGtype().typ == G_STRING || conversion.expr.getGtype().relation.gtype.typ == G_STRING, rhs.token(), "must be a string type, but got "+conversion.expr.getGtype().String())
		stringVarname, ok := conversion.expr.(*Relation)
		assert(ok, rhs.token(), "ok")
		stringVariable := stringVarname.expr.(*ExprVariable)
		stringVariable.emit()
		emit("push %%rax")
		strlen := stringVariable.getGtype().length
		emit("push $%d", strlen) // len
		emit("push $%d", strlen) // cap

	default:
		rhs.emit() // it should put values to rax,rbx,rcx
		emit("push %%rax")
		emit("push %%rbx")
		emit("push %%rcx")
	}

	emitSaveSlice(lhs, 0)
}

func (variable *ExprVariable) saveSlice(offset int) {
	emit("# *ExprVariable.saveSlice()")
	emit("pop %%rax")
	variable.emitOffsetSave(8, offset+ptrSize+sliceOffsetForLen)
	emit("pop %%rax")
	variable.emitOffsetSave(8, offset+ptrSize)
	emit("pop %%rax")
	variable.emitOffsetSave(8, offset)
}

func (variable *ExprVariable) saveInterface(offset int) {
	emit("# *ExprVariable.saveInterface()")
	emit("pop %%rax")
	variable.emitOffsetSave(8, offset+ptrSize+ptrSize)
	emit("pop %%rax")
	variable.emitOffsetSave(8, offset+ptrSize)
	emit("pop %%rax")
	variable.emitOffsetSave(8, offset)
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
	assert(rhs == nil || rhs.getGtype().typ == G_ARRAY, nil, "rhs should be array")
	switch {
	case elementType.typ == G_REL && elementType.relation.gtype.typ == G_STRUCT:
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
	default: // prrimitive type
		for i := 0; i < arrayType.length; i++ {
			offsetByIndex := i * elmSize
			switch rhs.(type) {
			case nil:
				// assign zero values
				emit("mov $0, %%rax")
			case *ExprArrayLiteral:
				arrayLiteral := rhs.(*ExprArrayLiteral)
				if elementType.getPrimType() == G_INTERFACE {
					if i >= len(arrayLiteral.values) {
						// zero value
						emit("push $0")
						emit("push $0")
						emit("push $0")
						emitSaveInterface(lhs, offsetByIndex)
						continue
					} else if arrayLiteral.values[i].getGtype().getPrimType() != G_INTERFACE {
						// conversion of dynamic type => interface type
						dynamicValue :=  arrayLiteral.values[i]
						convertDynamicTypeToInterface(dynamicValue)
						emitSaveInterface(lhs, offsetByIndex)
						continue
					} else {
						TBI(lhs.token(),"")
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

// for local var
func (decl *DeclVar) emit() {
	gtype := decl.variable.gtype
	switch {
	case gtype.typ == G_ARRAY:
		assignToArray(decl.varname, decl.initval)
	case gtype.typ == G_SLICE:
		assignToSlice(decl.varname, decl.initval)
	case gtype.typ == G_REL && gtype.relation.gtype.typ == G_STRUCT:
		assignToStruct(decl.varname, decl.initval)
	case gtype.typ == G_MAP:
		assignToMap(decl.varname, decl.initval)
	case gtype.typ == G_REL && gtype.relation.gtype.typ == G_INTERFACE:
		assignToInterface(decl.varname, decl.initval)
	default:
		assert(decl.variable.getGtype().getSize() <= 8, decl.token(), "invalid type:"+ gtype.String())
		// primitive types like int,bool,byte
		rhs := decl.initval
		if rhs == nil {
			// assign zero value
			rhs = &ExprNumberLiteral{}
		}
		rhs.emit()
		emitLsave(decl.variable.getGtype().getSize(), decl.variable.offset)
	}
}

func (decl *DeclType) emit() {
	// nothing to do
}

func (decl *DeclConst) emit() {
	// nothing to do
}

func (ast *StmtSatementList) emit() {
	for _, stmt := range ast.stmts {
		stmt.emit()
	}
}

func emitCollectIndexSave(array Expr, index Expr, offset int) {
	assert(array.getGtype().typ == G_ARRAY, array.token(), "should be array")
	elmType := array.getGtype().elementType
	emit("push %%rax # STACK 1 : the value") // stash value

	emit("# array.emit()")
	array.emit()                 // emit address
	emit("push %%rax # STACK 2") // store address of variable

	index.emit()
	emit("mov %%rax, %%rcx") // index

	size := elmType.getSize()
	assert(size > 0, nil, "size > 0")
	emit("mov $%d, %%rax    # size of one element", size)
	emit("imul %%rcx, %%rax # index * size")
	emit("push %%rax        # STACK 3 : store index * size")
	emit("pop %%rcx         # STACK 3: load  index * size")
	emit("pop %%rbx         # STACK 2 : load address of variable")
	emit("add %%rcx , %%rbx # (index * size) + address")
	if offset > 0 {
		emit("add $%d,  %%rbx # offset", offset)
	}
	emit("pop %%rax # STACK 1: restore the value")
	emit("mov %%rax, (%%rbx) # save the value")
	emit("")
}

func loadCollectIndex(array Expr, index Expr, offset int) {
	emit("# loadCollectIndex")
	if array.getGtype().typ == G_ARRAY {
		elmType := array.getGtype().elementType
		emit("# array.emit()")
		array.emit()       // emit address
		emit("push %%rax") // store address of variable

		index.emit()
		emit("mov %%rax, %%rcx") // index

		size := elmType.getSize()
		assert(size > 0, nil, "size > 0")
		emit("mov $%d, %%rax", size) // size of one element
		emit("imul %%rcx, %%rax")    // index * size
		emit("push %%rax")           // store index * size
		emit("pop %%rcx")            // load  index * size
		emit("pop %%rbx")            // load address of variable
		emit("add %%rcx , %%rbx")    // (index * size) + address
		if offset > 0 {
			emit("add $%d,  %%rbx", offset)
		}
		emit("mov (%%rbx), %%rax") // dereference the content of an emelment
	} else if array.getGtype().typ == G_SLICE {
		elmType := array.getGtype().elementType
		emit("# emit address of the low index")
		array.emit()       // eval pointer value
		emit("push %%rax") // store head address

		index.emit() // index
		emit("mov %%rax, %%rcx")

		size := elmType.getSize()
		assert(size > 0, nil, "size > 0")
		emit("mov $%d, %%rax", size) // size of one element
		emit("imul %%rcx, %%rax")    // set e.index * size => %rax
		emit("pop %%rbx")            // load head address
		emit("add %%rax , %%rbx")    // (e.index * size) + head address
		if offset > 0 {
			emit("add $%d,  %%rbx", offset)
		}
		emit("mov (%%rbx), %%rax") // dereference the content of an emelment

	} else if array.getGtype().typ == G_MAP {
		// e.g. x[key]
		emit("# emit map index expr")
		emit("# r10: map header address")
		emit("# r11: map len")
		emit("# r12: specified index value")
		emit("# r13: loop counter")

		// rax: found value (zero if not found)
		// rcx: ok (found: address of the index,  not found:0)
		_map := array
		emit("# emit mapData head address")
		_map.emit()
		emit("mov %%rax, %%r10 # copy head address")
		emitOffsetLoad(_map, IntSize, IntSize)
		emit("mov %%rax, %%r11 # copy len ")
		index.emit()
		emit("mov %%rax, %%r12 # index value")
		emitMapGet(array.getGtype())
	} else {
		TBI(array.token(), "unable to handle %s", array.getGtype())
	}
}

func emitMapGet(mapType *Gtype) {
	emit("# emitMapGet")
	emit("mov $0, %%r13 # init loop counter") // i = 0

	labelBegin := makeLabel()
	labelEnd := makeLabel()
	emit("%s: # begin loop ", labelBegin)

	labelIncr := makeLabel()
	// break if i < len
	emit("cmp %%r11, %%r13") // len > i
	emit("setl %%al")
	emit("movzb %%al, %%eax")
	emit("test %%rax, %%rax")
	emit("mov $0, %%rax") // key not found. set zero value.
	emit("mov $0, %%rcx") // ok = false
	emit("je %s  # jump if false", labelEnd)

	// check if index value matches
	emit("mov %%r13, %%rax")   // i
	emit("imul $16, %%rax")    // i * 16
	emit("mov %%r10, %%rcx")   // head
	emit("add %%rax, %%rcx")   // head + i * 16
	emit("mov (%%rcx), %%rax") // emit index address

	mapKeyType := mapType.mapKey
	mapValueType := mapType.mapValue
	if !mapKeyType.isString() {
		emit("mov (%%rax), %%rax") // dereference
	}
	if mapKeyType.isString() {
		emit("push %%r13")
		emit("push %%r11")
		emit("push %%r10")
		emitStringsEqual("%r12", "%rax")
		emit("pop %%r10")
		emit("pop %%r11")
		emit("pop %%r13")
	} else {
		// primitive comparison
		emit("cmp %%r12, %%rax # compare specifiedvalue vs indexvalue")
		emit("sete %%al")
		emit("movzb %%al, %%eax")
	}

	emit("test %%rax, %%rax")
	emit("je %s  # jump if false", labelIncr)

	emit("# Value found!")
	emit("mov 8(%%rcx), %%rax # set the found value address")
	if !mapValueType.isString() {
		emit("mov (%%rax), %%rax # dereference")
	}
	emit("jmp %s", labelEnd)

	emit("%s: # incr", labelIncr)
	emit("add $1, %%r13") // i++
	emit("jmp %s", labelBegin)

	emit("%s: # end loop", labelEnd)
}

func (e *ExprIndex) emit() {
	emit("# emit *ExprIndex")
	loadCollectIndex(e.collection, e.index, 0)
}

func (e *ExprNilLiteral) emit() {
	emit("mov $0, %%rax # nil literal")
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
	emit("mov $1, %%rax") // emit 1 for now.  @FIXME
}

func (e *ExprSlice) emit() {
	e.emitToStack()
	emit("pop %%rcx")
	emit("pop %%rbx")
	emit("pop %%rax")
}

func (e *ExprSlice) emitToStack() {
	emit("# assign to a slice")
	emit("#   emit address of the array")
	e.collection.emit()
	emit("push %%rax") // head of the array
	emit("#   emit low index")
	e.low.emit()
	emit("mov %%rax, %%rcx") // low index
	elmType := e.collection.getGtype().elementType
	size := elmType.getSize()
	assert(size > 0, nil, "size > 0")
	emit("mov $%d, %%rax", size) // size of one element
	emit("imul %%rcx, %%rax")    // index * size
	emit("pop %%rcx")            // head of the array
	emit("add %%rcx , %%rax")    // (index * size) + address
	emit("push %%rax")

	emit("#   calc and set len")

	if e.high == nil {
		e.high = &ExprNumberLiteral{
			val: e.collection.getGtype().length,
		}
	}
	calcLen := &ExprBinop{
		op:    "-",
		left:  e.high,
		right: e.low,
	}
	calcLen.emit()
	emit("push %%rax")

	emit("#   calc and set cap")
	calcCap := &ExprBinop{
		op: "-",
		left: &ExprNumberLiteral{
			val: e.collection.getGtype().length,
		},
		right: e.low,
	}
	calcCap.emit()
	emit("push %%rax")
}

func (e ExprArrayLiteral) emit() {
	errorft(e.token(), "DO NOT EMIT")
}

// https://golang.org/ref/spec#Type_assertions
func (e *ExprTypeAssertion) emit() {
	assert(e.expr.getGtype().getPrimType() == G_INTERFACE, e.token(), "expr must be an Interface type")
	if e.gtype.getPrimType() == G_INTERFACE {
		TBI(e.token(),"")
	} else {
		// if T is not an interface type,
		// x.(T) asserts that the dynamic type of x is identical to the type T.

		e.expr.emit() // emit interface
		// rax(ptr), rbx(typeId of method table), rcx(hashed typeId)
		emit("push %%rax")
		// @TODO DRY with type swtich statement
		typeLabel := groot.getTypeLabel(e.gtype)
		emit("lea .%s(%%rip), %%rax # type: %s", typeLabel, e.gtype)
		emitStringsEqual("%rax", "%rcx")

		emit("mov %%rax, %%rbx") // move flag
		// @TODO consider big data like slice, struct, etd
		emit("pop %%rax") // load ptr
		emit("mov (%%rax), %%rax") // deref
	}
}

func (ast *StmtContinue) emit() {
	panic("implement me")
}

func (ast *StmtBreak) emit() {
	panic("implement me")
}

func (ast *StmtExpr) emit() {
	ast.expr.emit()
}

func (ast *StmtDefer) emit() {
	panic("implement me")
}

func (e *ExprVaArg) emit() {
	panic("implement me " + e.tok.String())
}

func (e *ExprConversion) emit() {
	e.expr.emit()
}

func (e *ExprStructLiteral) emit() {
	errorft(e.token(), "This cannot be emitted alone")
}

func (e *ExprTypeSwitchGuard) emit() {
	e.expr.emit()
	emit("mov %%rcx, %%rax # copy type id")
}

func (e *ExprMapLiteral) emit() {
	TBI(e.token(), "")
}

func (ast *ExprMethodcall) getUniqueName() string {
	gtype := ast.receiver.getGtype()
	return getMethodUniqueName(gtype, ast.fname)
}

func (methodCall *ExprMethodcall) getOrigType() *Gtype {
	gtype := methodCall.receiver.getGtype()
	assertNotNil(methodCall.receiver !=nil, methodCall.token())
	assertNotNil(gtype != nil, methodCall.tok)
	assert(gtype.typ == G_REL || gtype.typ == G_POINTER || gtype.typ == G_INTERFACE, methodCall.tok, "method must be an interface or belong to a named type")
	var typeToBeloing *Gtype
	if gtype.typ == G_POINTER {
		typeToBeloing = gtype.origType
		assert(typeToBeloing != nil, methodCall.token(), "shoudl not be nil:" +  gtype.String())
	} else {
		typeToBeloing = gtype
	}
	assert(typeToBeloing.typ == G_REL, methodCall.tok, "method must belong to a named type")
	origType := typeToBeloing.relation.gtype
	//debugf("origType = %v", origType)
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
	return funcall.getFuncDef().rettypes
}

func (methodCall *ExprMethodcall) getRettypes() []*Gtype {
	origType := methodCall.getOrigType()
	if origType.typ == G_INTERFACE {
		return origType.imethods[methodCall.fname].rettypes
	} else {
		funcref, ok := origType.methods[methodCall.fname]
		if !ok {
			errorft(methodCall.token(), "method %s is not found in type %s", methodCall.fname, methodCall.receiver.getGtype())
		}
		return funcref.funcdef.rettypes
	}
}

type IrInterfaceMethodCall struct {
	receiver Expr
	methodName identifier
}

func (call *IrInterfaceMethodCall) emit(args []Expr) {
	emit("# emitCall %s", call.methodName)
	if true {
		mapType := &Gtype{
			typ:      G_MAP,
			mapKey:   &Gtype{
				typ: G_STRING,
			},
			mapValue: &Gtype{
				typ: G_STRING,
			},
		}
		emit("# emit typeId")
		emitOffsetLoad(call.receiver, ptrSize, ptrSize)
		emit("imul $8, %%rax")
		emit("push %%rax")
		emit("lea namedTypes(%%rip), %%rax")
		emit("pop %%rcx")
		emit("add %%rcx, %%rax")
		emit("# find method %s", call.methodName)
		emit("mov (%%rax), %%r10") // address of namedTypeN

		emit("mov $128, %%rax")                  // copy len
		emit("mov %%rax, %%r11")                  // copy len

		emit("lea .M%s, %%rax", call.methodName) // index value
		emit("mov %%rax, %%r12") // index value
		emitMapGet(mapType)
	}

	emit("push %%rax")

	emit("# setting arguments %v", args)

	receiver := args[0]
	emit("mov $0, %%rax")
	receiverType := receiver.getGtype()
	assert(receiverType.typ == G_REL && receiverType.relation.gtype.typ == G_INTERFACE, nil, "should be interface")

	// dereference: convert an interface value to a concrete value
	receiver.emit()

	emit("mov (%%rax), %%rax")

	emit("push %%rax  # receiver")

	otherArgs := args[1:]
	for i, arg := range otherArgs {
		if _, ok := arg.(*ExprVaArg); ok {
			// skip VaArg for now
			emit("mov $0, %%rax")
		} else {
			arg.emit()
		}
		emit("push %%rax  # argument no %d", i+2)
	}

	for i, _ := range args {
		j := len(args) - 1 - i
		emit("pop %%%s   # argument no %d", RegsForCall[j], j+1)
	}


	emit("pop %%rax")
	emit("call *%%rax")
}

func (methodCall *ExprMethodcall) emitInterfaceMethodCall() {
	args := []Expr{methodCall.receiver}
	for _, arg := range methodCall.args {
		args = append(args, arg)
	}
	call := &IrInterfaceMethodCall{
		receiver: methodCall.receiver,
		methodName : methodCall.fname,
	}
	call.emit(args)
}

func (methodCall *ExprMethodcall) emit() {
	origType := methodCall.getOrigType()
	if origType.typ == G_INTERFACE {
		methodCall.emitInterfaceMethodCall()
		return
	}

	args := []Expr{methodCall.receiver}
	for _, arg := range methodCall.args {
		args = append(args, arg)
	}

	funcref, ok := origType.methods[methodCall.fname]
	if !ok {
		errorft(methodCall.token(), "method %s is not found in type %s", methodCall.fname, methodCall.receiver.getGtype())
	}
	pkgname := funcref.funcdef.pkg
	name := methodCall.getUniqueName()
	var staticCall IrStaticCall =  getPackagedFuncName(pkgname, name)
	staticCall.emit(args)
}

func (funcall *ExprFuncallOrConversion) getFuncDef() *DeclFunc {
	relexpr := funcall.rel.expr
	assertNotNil2(relexpr, funcall.tok, funcall.rel)
	funcref, ok := relexpr.(*ExprFuncRef)
	if !ok {
		errorft(funcall.token(), "Compiler error: funcref is not *ExprFuncRef but %v", funcref, funcall.fname)
	}
	assertNotNil(funcref.funcdef != nil, nil)
	return funcref.funcdef
}

type ExprLen struct {
	tok *Token
	arg Expr
}

func (e *ExprLen) token() *Token {
	panic("implement me")
}

func (e *ExprLen) dump() {
	panic("implement me")
}

func (e *ExprLen) getGtype() *Gtype {
	return gInt
}

func (e *ExprLen) emit() {
	emit("# emit len()")
	arg := e.arg
	gtype := arg.getGtype()
	switch {
	case gtype.typ == G_ARRAY:
		emit("mov $%d, %%rax", gtype.length)
	case gtype.typ == G_SLICE:
		switch arg.(type) {
		case *Relation:
			emit("# Relation")
			emitOffsetLoad(arg, 8, ptrSize)
		case *ExprStructField:
			emit("# ExprStructField")
			emitOffsetLoad(arg, 8, ptrSize)
		case *ExprSliceLiteral:
			emit("# ExprSliceLiteral")
			length := len(arg.(*ExprSliceLiteral).values)
			emit("mov $%d, %%rax", length)
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
	case gtype.typ == G_MAP:
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
	case gtype.typ == G_STRING, gtype.typ == G_REL && gtype.relation.gtype.typ == G_STRING:
		TBI(arg.token(), "unable to handle %s", gtype)
	default:
		TBI(arg.token(), "unable to handle %s", gtype)
	}
}

func (funcall *ExprFuncallOrConversion) emit() {
	if funcall.rel.expr == nil && funcall.rel.gtype != nil {
		// Conversion
		conversion := &ExprConversion{
			tok: funcall.token(),
			gtype: funcall.rel.gtype,
			expr: funcall.args[0],
		}
		conversion.emit()
		return
	}

	decl := funcall.getFuncDef() // check existance
	if decl == nil {
		errorft(funcall.token(), "funcdef not found for funcall %s, rel=%v ", funcall.fname, funcall.rel)
	}

	// len()
	if decl == builinLen {
		assert(len(funcall.args) == 1, funcall.token(), "invalid arguments for len()")
		arg := funcall.args[0]
		exprLen := &ExprLen{
			tok: arg.token(),
			arg: arg,
		}
		exprLen.emit()
		return
	}
	var staticCall IrStaticCall = getPackagedFuncName(decl.pkg, funcall.fname)
	staticCall.emit(funcall.args)
}

type IrStaticCall string

func (ircall IrStaticCall) emit(args []Expr) {
	// nothing to do
	emit("# emitCall %s", ircall)

	emit("# setting arguments %v", args)

	for i, arg := range args {
		if _, ok := arg.(*ExprVaArg); ok {
			// skip VaArg for now
			emit("mov $0, %%rax")
		} else {
			arg.emit()
		}
		emit("push %%rax  # argument no %d", i+1)
	}

	for i, _ := range args {
		j := len(args) - 1 - i
		emit("pop %%%s   # argument no %d", RegsForCall[j], j+1)
	}

	emit("mov $0, %%rax")
	emit("call %s", ircall)
}

func (f *DeclFunc) emit() {
	f.emitPrologue()
	f.body.emit()
	emit("mov $0, %%rax")
	emitFuncEpilogue()
}

func evalIntExpr(e Expr) int {
	switch e.(type) {
	case nil:
		errorf("e is nil")
	case *ExprNumberLiteral:
		return e.(*ExprNumberLiteral).val
	case *ExprVariable:
		errorft(e.token(), "variable cannot be inteppreted at compile time")
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
		return evalIntExpr(e.(*ExprConstVariable).val)
	default:
		errorft(e.token(), "unkown type %T", e)
	}
	return 0
}

// gloabal var which should be initialized with zeros
// https://en.wikipedia.org/wiki/.bss
func (decl *DeclVar) emitBss() {
	// https://sourceware.org/binutils/docs-2.30/as/Lcomm.html#Lcomm
	emit(".lcomm %s, %d", decl.variable.varname, decl.variable.getGtype().getSize())
}

func (e *ExprStructLiteral) lookup(fieldname identifier) Expr {
	for _, field := range e.fields {
		if field.key == fieldname {
			return field.value
		}
	}

	return nil
}

func emitGlobalDeclInit(ptok *Token /* left type */, gtype *Gtype, value /* nullable */ Expr, containerName string) {
	if gtype.typ == G_ARRAY {
		arrayliteral, ok := value.(*ExprArrayLiteral)
		var values []Expr
		if ok {
			values = arrayliteral.values
		}
		assert(ok || arrayliteral == nil, ptok, fmt.Sprintf("*ExprArrayLiteral expected, but got %T", value))
		elmType := gtype.elementType
		assertNotNil(elmType != nil, nil)
		for i := 0; i < gtype.length; i++ {
			selector := fmt.Sprintf("%s[%d]", containerName, i)
			if i >= len(values) {
				// zero value
				emitGlobalDeclInit(ptok, elmType, nil, selector)
			} else {
				value := arrayliteral.values[i]
				assertNotNil(value != nil, nil)
				size := elmType.getSize()
				if size == 8 {
					if value.getGtype().typ == G_STRING {
						stringLiteral, ok := value.(*ExprStringLiteral)
						assert(ok, nil, "ok")
						emit(".quad .%s # %s", stringLiteral.slabel)
					} else {
						emit(".quad %d # %s %s", evalIntExpr(value), value.getGtype(), selector)
					}
				} else if size == 1 {
					emit(".byte %d", evalIntExpr(value))
				} else {
					emitGlobalDeclInit(ptok, gtype.elementType, value, selector)
				}
			}
		}
	} else if gtype.typ == G_SLICE {
		switch value.(type) {
		case *ExprSliceLiteral:
			// initialize a hidden array
			lit := value.(*ExprSliceLiteral)
			lit.invisiblevar.varname = identifier(fmt.Sprintf("$hiddenArray$%d", getHidddenArrayId()))
			emit(".quad %s", lit.invisiblevar.varname)      // address of the hidden array
			emit(".quad %d", lit.invisiblevar.gtype.length) // len
			emit(".quad %d", lit.invisiblevar.gtype.length) // cap
			arrayLiteral := &ExprArrayLiteral{
				gtype:  lit.invisiblevar.gtype,
				values: lit.values,
			}
			arrayDecl := &DeclVar{
				tok:      ptok,
				variable: lit.invisiblevar,
				initval:  arrayLiteral,
			}
			arrayDecl.emitGlobal()

		default:
			TBI(ptok, "unable to handle %s", gtype)
		}
	} else if gtype.typ == G_MAP {
		// @TODO
		emit(".quad 0")
		emit(".quad 0")
		emit(".quad 0")
	} else if gtype == gBool || (gtype.typ == G_REL && gtype.relation.gtype == gBool) {
		if value == nil {
			// zero value
			emit(".quad %d # %s %s", 0, gtype, containerName)
			return
		}
		val := evalIntExpr(value)
		emit(".quad %d # %s %s", val, gtype, containerName)
	} else if gtype.typ == G_REL && gtype.relation.gtype.typ == G_STRUCT {
		containerName = containerName + "." + string(gtype.relation.name)
		gtype.relation.gtype.calcStructOffset()
		for _, field := range gtype.relation.gtype.fields {
			if value == nil {
				emitGlobalDeclInit(ptok, field, nil, containerName+"."+string(field.fieldname))
				continue
			}
			structLiteral, ok := value.(*ExprStructLiteral)
			assert(ok, nil, "ok:"+containerName)
			value := structLiteral.lookup(field.fieldname)
			if value == nil {
				// zero value
				//continue
			}
			gtype := field
			emitGlobalDeclInit(ptok, gtype, value, containerName+"."+string(field.fieldname))
		}
	} else {
		var val int
		switch value.(type) {
		case nil:
			emit(".quad %d # %s %s zero value", 0, gtype, containerName)
		case *ExprNumberLiteral:
			val = value.(*ExprNumberLiteral).val
			emit(".quad %d # %s %s", val, gtype, containerName)
		case *ExprConstVariable:
			val = evalIntExpr(value)
			emit(".quad %d # %s ", val, gtype)
		case *ExprStringLiteral:
			stringLiteral := value.(*ExprStringLiteral)
			emit(".quad .%s", stringLiteral.slabel)
			value.getGtype().length = len(stringLiteral.val)
		case *Relation:
			emit(".quad 0 # TBI") // TBI
		case *ExprUop:
			uop := value.(*ExprUop)
			assert(uop.op == "&", ptok, "only uop & is allowed")
			emit(".quad hiddenStruct1") // @TODO
		default:
			TBI(ptok, "unable to handle %T", value)
		}
	}
}

func (decl *DeclVar) emitGlobal() {
	assert(decl.variable.isGlobal, nil, "should be global")
	assertNotNil(decl.variable.gtype != nil, nil)

	if decl.initval == nil {
		decl.emitBss()
		return
	}

	ptok := decl.token()
	gtype := decl.variable.gtype
	right := decl.initval

	emitLabel("%s: # %s", decl.variable.varname, gtype)
	emitGlobalDeclInit(ptok, gtype, right, "")
}

type IrRoot struct {
	vars           []*DeclVar
	funcs          []*DeclFunc
	stringLiterals []*ExprStringLiteral
	methodTable    map[int][]string
	hashedTypes    map[string]int
}

var groot *IrRoot

func (root *IrRoot) getTypeLabel(gtype *Gtype) string {
	dtypeId := root.hashedTypes[gtype.String()]
	label := fmt.Sprintf("DT%d", dtypeId)
	return label
}

func (root *IrRoot) emit() {
	groot = root
	// generate code
	emit(".data")

	emit("")
	emitComment("STRING LITERALS")
	for id, ast := range root.stringLiterals {
		ast.slabel = fmt.Sprintf("S%d", id)
		emitLabel(".%s:", ast.slabel)
		// https://sourceware.org/binutils/docs-2.30/as/String.html#String
		// the assembler marks the end of each string with a 0 byte.
		emit(".string \"%s\"", ast.val)
	}

	emit("")
	emitComment("Dinamic Types")
	var dtypeId int // 0 means nil
	for hashedType, _ := range root.hashedTypes {
		dtypeId++
		root.hashedTypes[hashedType] = dtypeId
		label := fmt.Sprintf("DT%d", dtypeId)
		emitLabel(".%s:", label)
		emit(".string \"%s\"", hashedType)
	}

	emitComment("Method table")

	emitLabel("%s:", "namedTypes")
	emit(".quad 0 # typeId:0")
	for i:= 1; i <= len(root.methodTable); i++ {
		emit(".quad namedType%d # typeId:%d", i, i)
	}

	var shortMethodNames map[string]string = make(map[string]string)

	for i:= 1; i <= len(root.methodTable); i++ {
		emitLabel("namedType%d:", i)
		for _, methodNameFull := range root.methodTable[i] {
			splitted := strings.Split(methodNameFull, "$")
			shortMethodName := splitted[1]
			emit(".quad .M%s # key", shortMethodName)
			emit(".quad %s # method", methodNameFull)
			shortMethodNames[shortMethodName] = shortMethodName
		}
	}

	emitComment("METHOD NAMES")
	for shortMethodName := range shortMethodNames {
		emitLabel(".M%s:", shortMethodName)
		emit(".string \"%s\"", shortMethodName)
	}

	emitComment("GLOBAL VARS")
	emit("")
	for _, vardecl := range root.vars {
		vardecl.emitGlobal()
	}

	emitComment("FUNCTIONS")
	emit(".text")
	for _, funcdecl := range root.funcs {
		funcdecl.emit()
	}
}
