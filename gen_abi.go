package main

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

var retRegi [14]string = [14]string{
	"rax", "rbx", "rcx", "rdx", "rdi", "rsi", "r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15",
}

var RegsForArguments [12]string = [12]string{"rdi", "rsi", "rdx", "rcx", "r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15"}

func (f *DeclFunc) emitPrologue() {
	emitWithoutIndent("%s:", f.getSymbol())
	emit("FUNC_PROLOGUE")

	var params []*ExprVariable

	// prepend receiver to params
	if f.receiver != nil {
		params = []*ExprVariable{f.receiver}
		for _, param := range f.params {
			params = append(params, param)
		}
	} else {
		params = f.params
	}

	// offset for params and local variables
	var offset int

	if len(params) > 0 {
		emit("# set params")
	}

	var regIndex int
	for _, param := range params {
		switch param.getGtype().getKind() {
		case G_SLICE, G_INTERFACE, G_MAP:
			offset -= IntSize * 3
			param.offset = offset
			emit("PUSH_ARG_%d # third", regIndex+2)
			emit("PUSH_ARG_%d # second", regIndex+1)
			emit("PUSH_ARG_%d # fist \"%s\" %s", regIndex, param.varname, param.getGtype().String())
			regIndex += sliceWidth
		default:
			offset -= IntSize
			param.offset = offset
			emit("PUSH_ARG_%d # param \"%s\" %s", regIndex, param.varname, param.getGtype().String())
			regIndex += 1
		}
	}

	if len(f.localvars) > 0 {
		emit("# Allocating stack for localvars len=%d", len(f.localvars))
	}

	var localarea int
	for _, lvar := range f.localvars {
		if lvar.gtype == nil {
			debugf("%s has nil gtype ", lvar)
		}
		size := lvar.gtype.getSize()
		assert(size != 0, lvar.token(), "size should  not be zero:"+lvar.gtype.String())
		loff := align(size, 8)
		localarea -= loff
		offset -= loff
		lvar.offset = offset
		//debugf("set offset %d to lvar %s, type=%s", lvar.offset, lvar.varname, lvar.gtype)
	}

	for i := len(f.localvars) - 1; i >= 0; i-- {
		lvar := f.localvars[i]
		emit("# offset %d variable \"%s\" %s", lvar.offset, lvar.varname, lvar.gtype.String())
	}

	if localarea != 0 {
		emit("sub $%d, %%rsp # total stack size", -localarea)
	}

	emitNewline()
}

func (ircall *IrStaticCall) emit(args []Expr) {
	// nothing to do
	emit("# emitCall %s", ircall.symbol)

	var numRegs int
	var param *ExprVariable
	var collectVariadicArgs bool // gather variadic args into a slice
	var variadicArgs []Expr
	var arg Expr
	var argIndex int
	for argIndex, arg = range args {
		var fromGtype string = ""
		if arg.getGtype() != nil {
			emit("# get fromGtype")
			fromGtype = arg.getGtype().String()
		}
		emit("# from %s", fromGtype)
		if argIndex < len(ircall.callee.params) {
			param = ircall.callee.params[argIndex]
			if param.isVariadic {
				if _, ok := arg.(*ExprVaArg); !ok {
					collectVariadicArgs = true
				}
			}
		}

		if collectVariadicArgs {
			variadicArgs = append(variadicArgs, arg)
			continue
		}

		var doConvertToInterface bool

		// do not convert receiver
		if !ircall.isMethodCall || argIndex != 0 {
			if param != nil && ircall.symbol != "printf" {
				emit("# has a corresponding param")

				var fromGtype *Gtype
				if arg.getGtype() != nil {
					fromGtype = arg.getGtype()
					emit("# fromGtype:%s", fromGtype.String())
				}

				var toGtype *Gtype
				if param.getGtype() != nil {
					toGtype = param.getGtype()
					emit("# toGtype:%s", toGtype.String())
				}

				if toGtype != nil && toGtype.getKind() == G_INTERFACE && fromGtype != nil && fromGtype.getKind() != G_INTERFACE {
					doConvertToInterface = true
				}
			}
		}

		if ircall.symbol == ".println" {
			doConvertToInterface = false
		}

		emit("# arg %d, doConvertToInterface=%s, collectVariadicArgs=%s",
			argIndex, bool2string(doConvertToInterface), bool2string(collectVariadicArgs))

		if doConvertToInterface {
			emit("# doConvertToInterface !!!")
			emitConversionToInterface(arg)
		} else {
			arg.emit()
		}

		var primType GTYPE_KIND = 0
		if arg.getGtype() != nil {
			primType = arg.getGtype().getKind()
		}
		var width int
		if doConvertToInterface || primType == G_INTERFACE {
			emit("PUSH_INTERFACE")
			width = interfaceWidth
		} else if primType == G_SLICE {
			emit("PUSH_SLICE")
			width = sliceWidth
		} else if primType == G_MAP {
			emit("PUSH_MAP")
			width = mapWidth
		} else {
			emit("PUSH_8")
			width = 1
		}
		numRegs += width
	}

	// check if callee has a variadic
	// https://golang.org/ref/spec#Passing_arguments_to_..._parameters
	// If f is invoked with no actual arguments for p, the value passed to p is nil.
	if !collectVariadicArgs {
		if argIndex+1 < len(ircall.callee.params) {
			param = ircall.callee.params[argIndex+1]
			if param.isVariadic {
				collectVariadicArgs = true
			}
		}
	}

	if collectVariadicArgs {
		emit("# collectVariadicArgs = true")
		lenArgs := len(variadicArgs)
		if lenArgs == 0 {
			emit("LOAD_EMPTY_SLICE")
			emit("PUSH_SLICE")
		} else {
			// var a []interface{}
			for vargIndex, varg := range variadicArgs {
				emit("# emit variadic arg")
				if vargIndex == 0 {
					emit("# make an empty slice to append")
					emit("LOAD_EMPTY_SLICE")
					emit("PUSH_SLICE")
				}
				// conversion : var ifc = x
				if varg.getGtype().getKind() == G_INTERFACE {
					varg.emit()
				} else {
					emitConversionToInterface(varg)
				}
				emit("PUSH_INTERFACE")
				emit("# calling append24")
				emit("POP_TO_ARG_5 # ifc_c")
				emit("POP_TO_ARG_4 # ifc_b")
				emit("POP_TO_ARG_3 # ifc_a")
				emit("POP_TO_ARG_2 # cap")
				emit("POP_TO_ARG_1 # len")
				emit("POP_TO_ARG_0 # ptr")
				emit("FUNCALL iruntime.append24")
				emit("PUSH_SLICE")
			}
		}
		numRegs += 3
	}

	for i := numRegs - 1; i >= 0; i-- {
		if i >= len(RegsForArguments) {
			errorft(args[0].token(), "too many arguments")
		}
		emit("POP_TO_ARG_%d", i)
	}

	emit("FUNCALL %s", ircall.symbol)
	emitNewline()
}

func (stmt *StmtReturn) emit() {
	if len(stmt.exprs) == 0 {
		// return void
		emit("mov $0, %%rax")
		stmt.emitDeferAndReturn()
		return
	}

	if len(stmt.exprs) > 7 {
		TBI(stmt.token(), "too many number of arguments")
	}

	var retRegiIndex int
	if len(stmt.exprs) == 1 {
		expr := stmt.exprs[0]
		rettype := stmt.rettypes[0]
		if rettype.getKind() == G_INTERFACE && expr.getGtype().getKind() != G_INTERFACE {
			if expr.getGtype() == nil {
				emit("LOAD_EMPTY_INTERFACE")
			} else {
				emitConversionToInterface(expr)
			}
		} else {
			expr.emit()
			if expr.getGtype() == nil && stmt.rettypes[0].kind == G_SLICE {
				emit("LOAD_EMPTY_SLICE")
			}
		}
		stmt.emitDeferAndReturn()
		return
	}
	for i, rettype := range stmt.rettypes {
		expr := stmt.exprs[i]
		expr.emit()
		//		rettype := stmt.rettypes[i]
		if expr.getGtype() == nil && rettype.kind == G_SLICE {
			emit("LOAD_EMPTY_SLICE")
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

	stmt.emitDeferAndReturn()
}



