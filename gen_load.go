// gen_load handles loading of expressions
package main

func (ast *ExprNumberLiteral) emit() {
	emit("LOAD_NUMBER %d", ast.val)
}

func (ast *ExprStringLiteral) emit() {
	emit("LOAD_STRING_LITERAL .%s", ast.slabel)
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
		if field.kind == G_ARRAY {
			variable.emitAddress(field.offset)
		} else {
			if variable.isGlobal {
				emit("LOAD_8_FROM_GLOBAL %s, %d+%d", variable.varname, field.offset,offset)
			} else {
				emit("LOAD_8_FROM_LOCAL %d+%d+%d", variable.offset, field.offset, offset)
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
	emit("ADD_NUMBER %d", field.offset)
}

func (a *ExprStructField) emit() {
	emit("# LOAD ExprStructField")
	a.calcOffset()
	switch a.strct.getGtype().kind {
	case G_POINTER: // pointer to struct
		strcttype := a.strct.getGtype().origType.relation.gtype
		field := strcttype.getField(a.fieldname)
		a.strct.emit()
		emit("ADD_NUMBER %d", field.offset)
		switch field.getKind() {
		case G_SLICE, G_INTERFACE, G_MAP:
			emit("LOAD_24_BY_DEREF")
		default:
			emit("LOAD_8_BY_DEREF")
		}

	case G_NAMED: // struct
		strcttype := a.strct.getGtype().relation.gtype
		assert(strcttype.size > 0, a.token(), "struct size should be > 0")
		field := strcttype.getField(a.fieldname)
		loadStructField(a.strct, field, 0)
	default:
		errorft(a.token(), "internal error: bad gtype %s", a.strct.getGtype().String())
	}
}

func (ast *ExprVariable) emit() {
	emit("# load variable \"%s\" %s", ast.varname, ast.getGtype().String())
	if ast.isGlobal {
		switch ast.gtype.getKind() {
		case G_INTERFACE:
			emit("LOAD_INTERFACE_FROM_GLOBAL %s", ast.varname)
		case G_SLICE:
			emit("LOAD_SLICE_FROM_GLOBAL %s", ast.varname)
		case G_MAP:
			emit("LOAD_MAP_FROM_GLOBAL %s", ast.varname)
		case G_ARRAY:
			ast.emitAddress(0)
		default:
			if ast.getGtype().getSize() == 1 {
				emit("LOAD_1_FROM_GLOBAL_CAST %s", ast.varname)
			} else {
				emit("LOAD_8_FROM_GLOBAL %s", ast.varname)
			}
		}
	} else {
		if ast.offset == 0 {
			errorft(ast.token(), "offset should not be zero for localvar %s", ast.varname)
		}
		switch ast.gtype.getKind() {
		case G_INTERFACE:
			emit("LOAD_INTERFACE_FROM_LOCAL %d", ast.offset)
		case G_SLICE:
			emit("LOAD_SLICE_FROM_LOCAL %d", ast.offset)
		case G_MAP:
			emit("LOAD_MAP_FROM_LOCAL %d", ast.offset)
		case G_ARRAY:
			ast.emitAddress(0)
		default:
			if ast.getGtype().getSize() == 1 {
				emit("LOAD_1_FROM_LOCAL_CAST %d", ast.offset)
			} else {
				emit("LOAD_8_FROM_LOCAL %d", ast.offset)
			}
		}
	}
}

func (variable *ExprVariable) emitAddress(offset int) {
	if variable.isGlobal {
		emit("LOAD_GLOBAL_ADDR %s, %d", variable.varname, offset)
	} else {
		if variable.offset == 0 {
			errorft(variable.token(), "offset should not be zero for localvar %s", variable.varname)
		}
		emit("LOAD_LOCAL_ADDR %d+%d", variable.offset, offset)
	}
}

func (rel *Relation) emit() {
	if rel.expr == nil {
		errorft(rel.token(), "rel.expr is nil: %s", rel.name)
	}
	rel.expr.emit()
}

func (ast *ExprConstVariable) emit() {
	emit("# *ExprConstVariable.emit() name=%s iotaindex=%d", ast.name, ast.iotaIndex)
	assert(ast.iotaIndex < 10000, ast.token(), "iotaindex is too large")
	assert(ast.val != nil, ast.token(), "const.val for should not be nil:"+string(ast.name))
	rel, ok := ast.val.(*Relation)
	if ok {
		emit("# rel=%s", rel.name)
		cnst, ok := rel.expr.(*ExprConstVariable)
		if ok && cnst == eIota {
			emit("# const is iota")
			// replace the iota expr by a index number
			val := &ExprNumberLiteral{
				val: ast.iotaIndex,
			}
			val.emit()
		} else {
			emit("# Not iota")
			ast.val.emit()
		}
	} else {
		emit("# const is not iota")
		ast.val.emit()
	}
}

func (ast *ExprUop) emit() {
	emit("# emitting ExprUop")
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
			ivv := e.invisiblevar
			assignToStruct(ivv, e)

			emitCallMalloc(e.getGtype().getSize())
			emit("PUSH_8")                     // to:ptr addr
			e.invisiblevar.emitAddress(0)
			emit("PUSH_8") // from:address of invisible var
			emitCopyStructFromStack(e.getGtype().getSize())
			// emit address
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
		emit("LOAD_8_BY_DEREF")
	} else if ast.op == "!" {
		ast.operand.emit()
		emit("CMP_EQ_ZERO")
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

func (variable *ExprVariable) emitOffsetLoad(size int, offset int) {
	assert(0 <= size && size <= 8, variable.token(), "invalid size")
	if variable.isGlobal {
		emit("LOAD_%d_FROM_GLOBAL %s %d", size, variable.varname, offset)
	} else {
		emit("LOAD_%d_FROM_LOCAL %d+%d", size,  variable.offset, offset)
	}
}

// rax: address
// rbx: len
// rcx: cap
func (e *ExprSliceLiteral) emit() {
	emit("# (*ExprSliceLiteral).emit()")
	length := len(e.values)
	//debugf("slice literal %s: underlyingarray size = %d (should be %d)", e.getGtype(), e.gtype.getSize(),  e.gtype.elementType.getSize() * length)
	emitCallMalloc(e.gtype.getSize() * length)
	emit("PUSH_8 # ptr")
	for i, value := range e.values {
		if e.gtype.elementType.getKind() == G_INTERFACE && value.getGtype().getKind() != G_INTERFACE {
			emitConversionToInterface(value)
		} else {
			value.emit()
		}

		emit("pop %%r10 # ptr")

		switch e.gtype.elementType.getKind() {
		case G_BYTE, G_INT, G_POINTER, G_STRING:
			emit("mov %%rax, %d(%%r10)", IntSize*i)
		case G_INTERFACE, G_SLICE, G_MAP:
			emit("mov %%rax, %d(%%r10)", IntSize*3*i)
			emit("mov %%rbx, %d(%%r10)", IntSize*3*i+ptrSize)
			emit("mov %%rcx, %d(%%r10)", IntSize*3*i+ptrSize+ptrSize)
		default:
			TBI(e.token(), "")
		}

		emit("push %%r10 # ptr")
	}

	emit("pop %%rax # ptr")
	emit("mov $%d, %%rbx # len", length)
	emit("mov $%d, %%rcx # cap", length)
}

func emitAddress(e Expr) {
	switch e.(type) {
	case *Relation:
		emitAddress(e.(*Relation).expr)
	case *ExprVariable:
		e.(*ExprVariable).emitAddress(0)
	default:
		TBI(e.token(), "")
	}
}

func emitOffsetLoad(lhs Expr, size int, offset int) {
	emit("# emitOffsetLoad(offset %d)", offset)
	switch lhs.(type) {
	case *Relation:
		rel := lhs.(*Relation)
		emitOffsetLoad(rel.expr, size, offset)
	case *ExprVariable:
		variable := lhs.(*ExprVariable)
		variable.emitOffsetLoad(size, offset)
	case *ExprStructField:
		structfield := lhs.(*ExprStructField)
		structfield.calcOffset()
		fieldType := structfield.getGtype()
		if structfield.strct.getGtype().kind == G_POINTER {
			structfield.strct.emit() // emit address of the struct
			emit("# offset %d + %d = %d", fieldType.offset, offset, fieldType.offset+offset)
			emit("ADD_NUMBER %d+%d", fieldType.offset,offset)
			//reg := getReg(size)
			emit("LOAD_8_BY_DEREF")
		} else {
			emitOffsetLoad(structfield.strct, size, fieldType.offset+offset)
		}
	case *ExprIndex:
		//  e.g. arrayLiteral.values[i].getGtype().getKind()
		indexExpr := lhs.(*ExprIndex)
		loadCollectIndex(indexExpr.collection, indexExpr.index, offset)
	case *ExprMethodcall:
		// @TODO this logic is temporarly. Need to be verified.
		mcall := lhs.(*ExprMethodcall)
		rettypes := mcall.getRettypes()
		assert(len(rettypes) == 1, lhs.token(), "rettype should be single")
		rettype := rettypes[0]
		assert(rettype.getKind() == G_POINTER, lhs.token(), "only pointer is supported")
		mcall.emit()
		emit("ADD_NUMBER %d", offset)
		emit("LOAD_8_BY_DEREF")
	default:
		errorft(lhs.token(), "unkonwn type %T", lhs)
	}
}

func loadArrayOrSliceIndex(collection Expr, index Expr, offset int) {
	elmType := collection.getGtype().elementType
	elmSize := elmType.getSize()
	assert(elmSize > 0, nil, "elmSize > 0")

	collection.emit()
	emit("PUSH_8 # head")

	index.emit()
	emit("IMUL_NUMBER %d", elmSize)
	emit("PUSH_8 # index * elmSize")

	emit("SUM_FROM_STACK # (index * elmSize) + head")
	emit("ADD_NUMBER %d", offset)

	primType := collection.getGtype().elementType.getKind()
	if primType == G_INTERFACE || primType == G_MAP || primType == G_SLICE {
		emit("LOAD_24_BY_DEREF")
	} else {
		// dereference the content of an emelment
		if elmSize == 1 {
			emit("LOAD_1_BY_DEREF")
		} else {
			emit("LOAD_8_BY_DEREF")
		}
	}
}

func loadCollectIndex(collection Expr, index Expr, offset int) {
	emit("# loadCollectIndex")
	if collection.getGtype().kind == G_ARRAY || collection.getGtype().kind == G_SLICE {
		loadArrayOrSliceIndex(collection, index, offset)
		return
	} else if collection.getGtype().getKind() == G_MAP {
		loadMapIndexExpr(collection, index)
	} else if collection.getGtype().getKind() == G_STRING {
		// https://golang.org/ref/spec#Index_expressions
		// For a of string type:
		//
		// a constant index must be in range if the string a is also constant
		// if x is out of range at run time, a run-time panic occurs
		// a[x] is the non-constant byte value at index x and the type of a[x] is byte
		// a[x] may not be assigned to
		emit("# load head address of the string")
		collection.emit()  // emit address
		emit("PUSH_8")
		index.emit()
		emit("PUSH_8")
		emit("SUM_FROM_STACK")
		emit("ADD_NUMBER %d", offset)
		emit("LOAD_8_BY_DEREF")
	} else {
		TBI(collection.token(), "unable to handle %s", collection.getGtype())
	}
}

func (e *ExprSlice) emitSubString() {
	// s[n:m]
	// new strlen: m - n
	var high Expr
	if e.high == nil {
		high = &ExprLen{
			tok: e.token(),
			arg: e.collection,
		}
	} else {
		high = e.high
	}
	eNewStrlen := &ExprBinop{
		tok:   e.token(),
		op:    "-",
		left:  high,
		right: e.low,
	}
	// mem size = strlen + 1
	eMemSize := &ExprBinop{
		tok:  e.token(),
		op:   "+",
		left: eNewStrlen,
		right: &ExprNumberLiteral{
			val: 1,
		},
	}

	// src address + low
	e.collection.emit()
	emit("PUSH_8")
	e.low.emit()
	emit("PUSH_8")
	emit("SUM_FROM_STACK")
	emit("PUSH_8")

	emitCallMallocDinamicSize(eMemSize)
	emit("PUSH_8")

	eNewStrlen.emit()
	emit("PUSH_8")

	emit("POP_TO_ARG_2")
	emit("POP_TO_ARG_1")
	emit("POP_TO_ARG_0")

	emit("FUNCALL iruntime.strcopy")
}

func (e *ExprSlice) emit() {
	if e.collection.getGtype().isString() {
		e.emitSubString()
	} else {
		e.emitSlice()
	}
}

func (e *ExprSlice) emitSlice() {
	elmType := e.collection.getGtype().elementType
	size := elmType.getSize()
	assert(size > 0, nil, "size > 0")

	emit("# assign to a slice")
	emit("#   emit address of the array")
	e.collection.emit()
	emit("PUSH_8 # head of the array")
	e.low.emit()
	emit("PUSH_8 # low index")
	emit("LOAD_NUMBER %d", size)
	emit("PUSH_8")
	emit("IMUL_FROM_STACK")
	emit("PUSH_8")
	emit("SUM_FROM_STACK")
	emit("PUSH_8")

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
	emit("PUSH_8")

	emit("#   calc and set cap")
	var max Expr
	if e.max != nil {
		max = e.max
	} else {
		max = &ExprCap{
			tok: e.token(),
			arg: e.collection,
		}
	}
	calcCap := &ExprBinop{
		op:    "-",
		left:  max,
		right: e.low,
	}

	calcCap.emit()

	emit("PUSH_8")
	emit("POP_SLICE")
}



