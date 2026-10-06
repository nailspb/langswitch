#include <ApplicationServices/ApplicationServices.h>
#include <Carbon/Carbon.h>
#include <dispatch/dispatch.h>
#include "_cgo_export.h"

// Метка событий, сгенерированных программой: по ней хук отличает их от нажатий пользователя.
static const int64_t injectedMark = 0x4C53;

enum { evKeyDown, evKeyUp, evFlags, evMouse };

static CFMachPortRef tap;

static CGEventRef tapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *ctx) {
	if (type == kCGEventTapDisabledByTimeout) {
		CGEventTapEnable(tap, true);
		return event;
	}
	int kind;
	switch (type) {
	case kCGEventKeyDown: kind = evKeyDown; break;
	case kCGEventKeyUp: kind = evKeyUp; break;
	case kCGEventFlagsChanged: kind = evFlags; break;
	case kCGEventLeftMouseDown:
	case kCGEventRightMouseDown:
	case kCGEventOtherMouseDown: kind = evMouse; break;
	default: return event;
	}
	int injected = CGEventGetIntegerValueField(event, kCGEventSourceUserData) == injectedMark;
	int code = (int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
	if (goHandleEvent(kind, code, CGEventGetFlags(event), injected)) {
		return NULL;
	}
	return event;
}

int lsTrusted(void) {
	const void *keys[] = { kAXTrustedCheckOptionPrompt };
	const void *values[] = { kCFBooleanTrue };
	CFDictionaryRef opts = CFDictionaryCreate(NULL, keys, values, 1,
		&kCFCopyStringDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	Boolean ok = AXIsProcessTrustedWithOptions(opts);
	CFRelease(opts);
	return ok;
}

int lsRunTap(void) {
	CGEventMask mask = CGEventMaskBit(kCGEventKeyDown) | CGEventMaskBit(kCGEventKeyUp) |
		CGEventMaskBit(kCGEventFlagsChanged) | CGEventMaskBit(kCGEventLeftMouseDown) |
		CGEventMaskBit(kCGEventRightMouseDown) | CGEventMaskBit(kCGEventOtherMouseDown);
	tap = CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap, kCGEventTapOptionDefault,
		mask, tapCallback, NULL);
	if (tap == NULL) {
		return 0;
	}
	CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
	CFRunLoopAddSource(CFRunLoopGetCurrent(), src, kCFRunLoopCommonModes);
	CFRelease(src);
	CGEventTapEnable(tap, true);
	CFRunLoopRun();
	return 1;
}

static void selectNextSource(void *ctx) {
	const void *keys[] = { kTISPropertyInputSourceCategory, kTISPropertyInputSourceIsSelectCapable };
	const void *values[] = { kTISCategoryKeyboardInputSource, kCFBooleanTrue };
	CFDictionaryRef filter = CFDictionaryCreate(NULL, keys, values, 2,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFArrayRef list = TISCreateInputSourceList(filter, false);
	CFRelease(filter);
	if (list == NULL) {
		return;
	}
	CFIndex n = CFArrayGetCount(list);
	TISInputSourceRef cur = TISCopyCurrentKeyboardInputSource();
	CFStringRef curID = TISGetInputSourceProperty(cur, kTISPropertyInputSourceID);
	CFIndex idx = -1;
	for (CFIndex i = 0; i < n; i++) {
		TISInputSourceRef s = (TISInputSourceRef)CFArrayGetValueAtIndex(list, i);
		if (CFEqual(TISGetInputSourceProperty(s, kTISPropertyInputSourceID), curID)) {
			idx = i;
			break;
		}
	}
	if (n > 0) {
		TISSelectInputSource((TISInputSourceRef)CFArrayGetValueAtIndex(list, (idx + 1) % n));
	}
	CFRelease(cur);
	CFRelease(list);
}

// TIS-функции безопасно вызывать только из главного потока.
void lsSwitchLayout(void) {
	dispatch_sync_f(dispatch_get_main_queue(), NULL, selectNextSource);
}

void lsPostKey(uint16_t code, int down, uint64_t flags) {
	CGEventRef ev = CGEventCreateKeyboardEvent(NULL, code, down);
	if (ev == NULL) {
		return;
	}
	CGEventSetFlags(ev, (CGEventFlags)flags);
	CGEventSetIntegerValueField(ev, kCGEventSourceUserData, injectedMark);
	CGEventPost(kCGHIDEventTap, ev);
	CFRelease(ev);
}

static CFArrayRef copyKeyboardSources(void) {
	const void *keys[] = { kTISPropertyInputSourceCategory, kTISPropertyInputSourceIsSelectCapable };
	const void *values[] = { kTISCategoryKeyboardInputSource, kCFBooleanTrue };
	CFDictionaryRef filter = CFDictionaryCreate(NULL, keys, values, 2,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFArrayRef list = TISCreateInputSourceList(filter, false);
	CFRelease(filter);
	return list;
}

typedef struct {
	const uint16_t *codes;
	int ncodes;
	uint16_t *out; // [раскладка][клавиша][shift]
	int maxLayouts;
	int *current;
	int count;
} layoutsCtx;

static void readLayouts(void *p) {
	layoutsCtx *c = p;
	*c->current = -1;
	CFArrayRef list = copyKeyboardSources();
	if (list == NULL) {
		return;
	}
	TISInputSourceRef cur = TISCopyCurrentKeyboardInputSource();
	CFStringRef curID = TISGetInputSourceProperty(cur, kTISPropertyInputSourceID);
	CFIndex n = CFArrayGetCount(list);
	if (n > c->maxLayouts) {
		n = c->maxLayouts;
	}
	for (CFIndex l = 0; l < n; l++) {
		TISInputSourceRef s = (TISInputSourceRef)CFArrayGetValueAtIndex(list, l);
		if (CFEqual(TISGetInputSourceProperty(s, kTISPropertyInputSourceID), curID)) {
			*c->current = (int)l;
		}
		CFDataRef data = TISGetInputSourceProperty(s, kTISPropertyUnicodeKeyLayoutData);
		if (data == NULL) {
			continue; // методы ввода (IME) без раскладки клавиш
		}
		const UCKeyboardLayout *kl = (const UCKeyboardLayout *)CFDataGetBytePtr(data);
		for (int i = 0; i < c->ncodes; i++) {
			for (int shift = 0; shift < 2; shift++) {
				UInt32 dead = 0;
				UniChar chars[4];
				UniCharCount len = 0;
				OSStatus st = UCKeyTranslate(kl, c->codes[i], kUCKeyActionDown,
					shift ? (shiftKey >> 8) & 0xFF : 0, LMGetKbdType(),
					kUCKeyTranslateNoDeadKeysBit, &dead, 4, &len, chars);
				c->out[(l * c->ncodes + i) * 2 + shift] = (st == noErr && len == 1) ? chars[0] : 0;
			}
		}
	}
	c->count = (int)n;
	CFRelease(cur);
	CFRelease(list);
}

int lsLayouts(const uint16_t *codes, int ncodes, uint16_t *out, int maxLayouts, int *current) {
	layoutsCtx c = { codes, ncodes, out, maxLayouts, current, 0 };
	dispatch_sync_f(dispatch_get_main_queue(), &c, readLayouts);
	return c.count;
}

static void selectSource(void *p) {
	int idx = *(int *)p;
	CFArrayRef list = copyKeyboardSources();
	if (list == NULL) {
		return;
	}
	if (idx >= 0 && idx < CFArrayGetCount(list)) {
		TISSelectInputSource((TISInputSourceRef)CFArrayGetValueAtIndex(list, idx));
	}
	CFRelease(list);
}

void lsSetLayout(int idx) {
	dispatch_sync_f(dispatch_get_main_queue(), &idx, selectSource);
}
