package main

import "fmt"

// gloabal var which should be initialized with zeros
// https://en.wikipedia.org/wiki/.bss
func (decl *DeclVar) emitBss() {
	emit(".data")
	// https://sourceware.org/binutils/docs-2.30/as/Lcomm.html#Lcomm
	emit(".lcomm %s, %d", decl.variable.varname, decl.variable.getGtype().getSize())
}

func (decl *DeclVar) emitData() {
	ptok := decl.token()
	gtype := decl.variable.gtype
	right := decl.initval

	emit("# emitData()")
	emit(".data 0")
	emitWithoutIndent("%s: # gtype=%s", decl.variable.varname, gtype.String())
	emit("# right.gtype = %s", right.getGtype().String())
	doEmitData(ptok, right.getGtype(), right, "", 0)
}

func (e *ExprStructLiteral) lookup(fieldname identifier) Expr {
	for _, field := range e.fields {
		if field.key == fieldname {
			return field.value
		}
	}

	return nil
}

func doEmitData(ptok *Token /* left type */, gtype *Gtype, value /* nullable */ Expr, containerName string, depth int) {
	emit("# doEmitData: containerName=%s, depth=%d", containerName, depth)
	primType := gtype.getKind()
	if primType == G_ARRAY {
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
				doEmitData(ptok, elmType, nil, selector, depth)
			} else {
				value := arrayliteral.values[i]
				assertNotNil(value != nil, nil)
				size := elmType.getSize()
				if size == 8 {
					if value.getGtype().kind == G_STRING {
						stringLiteral, ok := value.(*ExprStringLiteral)
						assert(ok, nil, "ok")
						emit(".quad .%s", stringLiteral.slabel)
					} else {
						switch value.(type) {
						case *ExprUop:
							uop := value.(*ExprUop)
							rel, ok := uop.operand.(*Relation)
							assert(ok, uop.token(), "only variable is allowed")
							emit(".quad %s # %s %s", rel.name, value.getGtype().String(), selector)
						case *Relation:
							assert(false, value.token(), "variable here is not allowed")
						default:
							emit(".quad %d # %s %s", evalIntExpr(value), value.getGtype().String(), selector)
						}
					}
				} else if size == 1 {
					emit(".byte %d", evalIntExpr(value))
				} else {
					doEmitData(ptok, gtype.elementType, value, selector, depth)
				}
			}
		}
		emit(".quad 0 # nil terminator")

	} else if primType == G_SLICE {
		switch value.(type) {
		case nil:
			return
		case *ExprSliceLiteral:
			// initialize a hidden array
			lit := value.(*ExprSliceLiteral)
			arrayLiteral := &ExprArrayLiteral{
				gtype:  lit.invisiblevar.gtype,
				values: lit.values,
			}

			emitDataAddr(arrayLiteral, depth)               // emit underlying array
			emit(".quad %d", lit.invisiblevar.gtype.length) // len
			emit(".quad %d", lit.invisiblevar.gtype.length) // cap
		default:
			TBI(ptok, "unable to handle gtype %s", gtype.String())
		}
	} else if primType == G_MAP || primType == G_INTERFACE {
		// @TODO
		emit(".quad 0")
		emit(".quad 0")
		emit(".quad 0")
	} else if primType == G_BOOL {
		if value == nil {
			// zero value
			emit(".quad %d # %s %s", 0, gtype.String(), containerName)
			return
		}
		val := evalIntExpr(value)
		emit(".quad %d # %s %s", val, gtype.String(), containerName)
	} else if primType == G_STRUCT {
		containerName = containerName + "." + string(gtype.relation.name)
		gtype.relation.gtype.calcStructOffset()
		for _, field := range gtype.relation.gtype.fields {
			emit("# padding=%d", field.padding)
			switch field.padding {
			case 1:
				emit(".byte 0 # padding")
			case 4:
				emit(".double 0 # padding")
			case 8:
				emit(".quad 0 # padding")
			default:
			}
			emit("# field:offesr=%d, fieldname=%s", field.offset, field.fieldname)
			if value == nil {
				doEmitData(ptok, field, nil, containerName+"."+string(field.fieldname), depth)
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
			doEmitData(ptok, gtype, value, containerName+"."+string(field.fieldname), depth)
		}
	} else {
		var val int
		switch value.(type) {
		case nil:
			emit(".quad %d # %s %s zero value", 0, gtype.String(), containerName)
		case *ExprNumberLiteral:
			val = value.(*ExprNumberLiteral).val
			emit(".quad %d # %s %s", val, gtype.String(), containerName)
		case *ExprConstVariable:
			cnst := value.(*ExprConstVariable)
			val = evalIntExpr(cnst)
			emit(".quad %d # %s ", val, gtype.String())
		case *ExprVariable:
			vr := value.(*ExprVariable)
			val = evalIntExpr(vr)
			emit(".quad %d # %s ", val, gtype.String())
		case *ExprBinop:
			val = evalIntExpr(value)
			emit(".quad %d # %s ", val, gtype.String())
		case *ExprStringLiteral:
			stringLiteral := value.(*ExprStringLiteral)
			emit(".quad .%s", stringLiteral.slabel)
		case *Relation:
			rel := value.(*Relation)
			doEmitData(ptok, gtype, rel.expr, "rel", depth)
		case *ExprUop:
			uop := value.(*ExprUop)
			assert(uop.op == "&", ptok, "only uop & is allowed")
			operand := uop.operand
			rel, ok := operand.(*Relation)
			if ok {
				assert(ok, value.token(), "operand should be *Relation")
				vr, ok := rel.expr.(*ExprVariable)
				assert(ok, value.token(), "operand should be a variable")
				assert(vr.isGlobal, value.token(), "operand should be a global variable")
				emit(".quad %s", vr.varname)
			} else {
				// var gv = &Struct{_}
				emitDataAddr(operand, depth)
			}
		default:
			TBI(ptok, "unable to handle %d", primType)
		}
	}
}

// this logic is stolen from 8cc.
func emitDataAddr(operand Expr, depth int) {
	emit(".data %d", depth+1)
	label := makeLabel()
	emit("%s:", label)
	doEmitData(nil, operand.getGtype(), operand, "", depth+1)
	emit(".data %d", depth)
	emit(".quad %s", label)
}

func (decl *DeclVar) emitGlobal() {
	emitWithoutIndent("# emitGlobal for %s", decl.variable.varname)
	assertNotNil(decl.variable.gtype != nil, nil)

	if decl.initval == nil {
		decl.emitBss()
	} else {
		decl.emitData()
	}
}


