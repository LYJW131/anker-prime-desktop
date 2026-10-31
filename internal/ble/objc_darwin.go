//go:build darwin

package ble

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The Objective-C helpers follow MyGo's internal/darwin/objc.go.

type id = objc.ID

var (
	loadOnce sync.Once
	loadErr  error

	libCB       uintptr
	msgSendAddr uintptr
	poolPushFn  uintptr
	poolPopFn   uintptr

	dispatchQueueCreate func(label *byte, attr uintptr) uintptr
	dispatchSyncF       func(queue, ctx, work uintptr)

	// syncTrampoline is the C function dispatch_sync_f runs: it looks up
	// the Go function registered under its context.
	syncTrampoline uintptr
	syncFns        sync.Map // uintptr -> *syncCall
	syncNext       atomic.Uintptr
)

type syncCall struct {
	fn    func()
	panic any
}

// load opens CoreBluetooth and registers the delegate classes, once.
func load() error {
	loadOnce.Do(func() {
		libObjC, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			loadErr = err
			return
		}
		libCB, err = purego.Dlopen("/System/Library/Frameworks/CoreBluetooth.framework/CoreBluetooth", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			loadErr = fmt.Errorf("ble: cannot load CoreBluetooth: %w", err)
			return
		}
		if msgSendAddr, err = purego.Dlsym(libObjC, "objc_msgSend"); err != nil {
			loadErr = err
			return
		}
		if poolPushFn, err = purego.Dlsym(libObjC, "objc_autoreleasePoolPush"); err != nil {
			loadErr = err
			return
		}
		if poolPopFn, err = purego.Dlsym(libObjC, "objc_autoreleasePoolPop"); err != nil {
			loadErr = err
			return
		}
		purego.RegisterLibFunc(&dispatchQueueCreate, purego.RTLD_DEFAULT, "dispatch_queue_create")
		purego.RegisterLibFunc(&dispatchSyncF, purego.RTLD_DEFAULT, "dispatch_sync_f")
		syncTrampoline = purego.NewCallback(func(ctx uintptr) {
			v, ok := syncFns.Load(ctx)
			if !ok {
				return
			}
			call := v.(*syncCall)
			defer func() { call.panic = recover() }()
			pool := poolPush()
			defer poolPop(pool)
			call.fn()
		})
		loadErr = registerDelegates()
	})
	return loadErr
}

// onQueue runs fn on queue and waits for it, inside an autorelease pool, so
// that calls into CoreBluetooth are serialized with its callbacks. It must
// not be called from a callback (the queue would deadlock) nor while holding
// a lock a callback takes. A panic in fn is re-raised in the caller.
func onQueue(queue uintptr, fn func()) {
	call := &syncCall{fn: fn}
	key := syncNext.Add(1)
	syncFns.Store(key, call)
	dispatchSyncF(queue, key, syncTrampoline)
	syncFns.Delete(key)
	if call.panic != nil {
		panic(call.panic)
	}
}

func poolPush() uintptr {
	p, _, _ := purego.SyscallN(poolPushFn)
	return p
}

func poolPop(p uintptr) { purego.SyscallN(poolPopFn, p) }

var (
	selMu    sync.RWMutex
	selCache = map[string]objc.SEL{}
)

func sel(name string) objc.SEL {
	selMu.RLock()
	s, ok := selCache[name]
	selMu.RUnlock()
	if ok {
		return s
	}
	s = objc.RegisterName(name)
	selMu.Lock()
	selCache[name] = s
	selMu.Unlock()
	return s
}

func class(name string) id {
	c := id(objc.GetClass(name))
	if c == 0 {
		panic("ble: Objective-C class " + name + " not found")
	}
	return c
}

// send sends a message whose arguments are integers or pointers. Go
// pointers converted in the call stay alive and in place until it returns
// (go:uintptrescapes).
//
//go:uintptrescapes
func send(obj id, selector string, args ...uintptr) id {
	var a [8]uintptr
	a[0], a[1] = uintptr(obj), uintptr(sel(selector))
	n := copy(a[2:], args)
	r, _, _ := purego.SyscallN(msgSendAddr, a[:n+2]...)
	return id(r)
}

//go:uintptrescapes
func sendBool(obj id, selector string, args ...uintptr) bool {
	return byte(send(obj, selector, args...)) != 0
}

//go:uintptrescapes
func sendInt(obj id, selector string, args ...uintptr) int {
	return int(send(obj, selector, args...))
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

func retain(obj id) id {
	if obj == 0 {
		return 0
	}
	return send(obj, "retain")
}

func release(obj id) {
	if obj != 0 {
		send(obj, "release")
	}
}

const nsUTF8StringEncoding = 4

// nsString returns an autoreleased NSString.
func nsString(s string) id {
	b := unsafe.StringData(s)
	str := send(send(class("NSString"), "alloc"), "initWithBytes:length:encoding:",
		uintptr(unsafe.Pointer(b)), uintptr(len(s)), nsUTF8StringEncoding)
	runtime.KeepAlive(s)
	return send(str, "autorelease")
}

func goString(str id) string {
	if str == 0 {
		return ""
	}
	p := send(str, "UTF8String")
	if p == 0 {
		return ""
	}
	n := sendInt(str, "lengthOfBytesUsingEncoding:", nsUTF8StringEncoding)
	return string(unsafe.Slice(*(**byte)(unsafe.Pointer(&p)), n))
}

// nsData returns an autoreleased NSData with a copy of b.
func nsData(b []byte) id {
	var p unsafe.Pointer
	if len(b) > 0 {
		p = unsafe.Pointer(&b[0])
	}
	d := send(class("NSData"), "dataWithBytes:length:", uintptr(p), uintptr(len(b)))
	runtime.KeepAlive(b)
	return d
}

func goBytes(data id) []byte {
	if data == 0 {
		return nil
	}
	n := sendInt(data, "length")
	if n == 0 {
		return []byte{}
	}
	p := send(data, "bytes")
	return append([]byte(nil), unsafe.Slice(*(**byte)(unsafe.Pointer(&p)), n)...)
}

func nsArray(objs []id) id {
	arr := send(class("NSMutableArray"), "arrayWithCapacity:", uintptr(len(objs)))
	for _, o := range objs {
		send(arr, "addObject:", uintptr(o))
	}
	return arr
}

func arrayItems(arr id) []id {
	if arr == 0 {
		return nil
	}
	n := sendInt(arr, "count")
	out := make([]id, n)
	for i := range n {
		out[i] = send(arr, "objectAtIndex:", uintptr(i))
	}
	return out
}

func isKind(obj id, className string) bool {
	return obj != 0 && sendBool(obj, "isKindOfClass:", uintptr(class(className)))
}

// cbString returns an NSString constant CoreBluetooth exports, falling back
// to its name, which is also its value for the advertisement keys.
func cbString(name, fallback string) id {
	p, err := purego.Dlsym(libCB, name)
	if err != nil || p == 0 {
		return nsString(fallback)
	}
	return **(**id)(unsafe.Pointer(&p))
}

// cbUUIDs returns an autoreleased NSArray of CBUUIDs, or 0 for none.
func cbUUIDs(uuids []string) id {
	if len(uuids) == 0 {
		return 0
	}
	objs := make([]id, len(uuids))
	for i, u := range uuids {
		objs[i] = send(class("CBUUID"), "UUIDWithString:", uintptr(nsString(shortUUID(u))))
	}
	return nsArray(objs)
}

// cbUUIDString returns a CBUUID's normalized UUID.
func cbUUIDString(u id) string {
	if u == 0 {
		return ""
	}
	return NormalizeUUID(goString(send(u, "UUIDString")))
}

// identifierOf returns a peripheral's identifier.
func identifierOf(p id) string {
	return goString(send(send(p, "identifier"), "UUIDString"))
}

// nsErr turns an NSError into a Go error, nil for none.
func nsErr(e id) error {
	if e == 0 {
		return nil
	}
	return fmt.Errorf("%s (%s %d)", goString(send(e, "localizedDescription")),
		goString(send(e, "domain")), sendInt(e, "code"))
}
