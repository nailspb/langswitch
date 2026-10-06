#include <X11/Xlib.h>
#include <X11/XKBlib.h>
#include <X11/extensions/record.h>
#include <X11/extensions/XTest.h>
#include "_cgo_export.h"

static Display *ctrl; // команды: XTest, Xkb, создание контекста XRecord
static Display *data; // поток событий XRecord
static Display *win;  // запросы об активном окне из потока XRecord

// ignoreErrors не даёт Xlib завершить процесс, если окно закрылось во время запроса.
static int ignoreErrors(Display *d, XErrorEvent *e) {
	return 0;
}

int lsOpen(void) {
	ctrl = XOpenDisplay(NULL);
	data = XOpenDisplay(NULL);
	win = XOpenDisplay(NULL);
	XSetErrorHandler(ignoreErrors);
	return ctrl != NULL && data != NULL && win != NULL;
}

static void recordCallback(XPointer priv, XRecordInterceptData *d) {
	if (d->category == XRecordFromServer && d->data != NULL) {
		unsigned char *p = d->data;
		goHandleEvent(p[0] & 0x7F, p[1]);
	}
	XRecordFreeData(d);
}

int lsRunRecord(void) {
	int major, minor;
	if (!XRecordQueryVersion(ctrl, &major, &minor)) {
		return 1;
	}
	XRecordRange *range = XRecordAllocRange();
	if (range == NULL) {
		return 2;
	}
	range->device_events.first = KeyPress;
	range->device_events.last = ButtonPress;
	XRecordClientSpec clients = XRecordAllClients;
	XRecordContext ctx = XRecordCreateContext(ctrl, 0, &clients, 1, &range, 1);
	XFree(range);
	if (ctx == 0) {
		return 3;
	}
	XSync(ctrl, False);
	if (!XRecordEnableContext(data, ctx, recordCallback, NULL)) { // блокируется
		return 4;
	}
	return 0;
}

int lsSwitchLayout(void) {
	XkbStateRec state;
	if (XkbGetState(ctrl, XkbUseCoreKbd, &state) != Success) {
		return 0;
	}
	XkbDescPtr desc = XkbAllocKeyboard();
	if (desc == NULL) {
		return 0;
	}
	int groups = 1;
	if (XkbGetControls(ctrl, XkbAllControlsMask, desc) == Success && desc->ctrls != NULL) {
		groups = desc->ctrls->num_groups;
	}
	XkbFreeKeyboard(desc, XkbAllComponentsMask, True);
	if (groups > 1) {
		XkbLockGroup(ctrl, XkbUseCoreKbd, (state.group + 1) % groups);
		XFlush(ctrl);
	}
	return 1;
}

void lsFakeKey(int code, int down) {
	XTestFakeKeyEvent(ctrl, code, down ? True : False, CurrentTime);
}

void lsFlush(void) {
	XFlush(ctrl);
}

#include <xkbcommon/xkbcommon.h>

static int numGroups(void) {
	XkbDescPtr desc = XkbAllocKeyboard();
	if (desc == NULL) {
		return 0;
	}
	int groups = 1;
	if (XkbGetControls(ctrl, XkbAllControlsMask, desc) == Success && desc->ctrls != NULL) {
		groups = desc->ctrls->num_groups;
	}
	XkbFreeKeyboard(desc, XkbAllComponentsMask, True);
	return groups;
}

// lsLayouts заполняет out[группа][клавиша][shift] символами Unicode (0 — нет символа).
int lsLayouts(const int *codes, int ncodes, uint32_t *out, int maxLayouts, int *current) {
	XkbStateRec state;
	if (XkbGetState(ctrl, XkbUseCoreKbd, &state) != Success) {
		return 0;
	}
	int groups = numGroups();
	if (groups > maxLayouts) {
		groups = maxLayouts;
	}
	for (int g = 0; g < groups; g++) {
		for (int i = 0; i < ncodes; i++) {
			for (int shift = 0; shift < 2; shift++) {
				KeySym ks = XkbKeycodeToKeysym(ctrl, codes[i], g, shift);
				out[(g * ncodes + i) * 2 + shift] = ks == NoSymbol ? 0 : xkb_keysym_to_utf32(ks);
			}
		}
	}
	*current = state.group;
	return groups;
}

void lsSetLayout(int group) {
	XkbLockGroup(ctrl, XkbUseCoreKbd, group);
	XFlush(ctrl);
}

// cardinalProp читает первое 32-битное значение свойства окна.
static int cardinalProp(Window w, Atom prop, unsigned long *out) {
	Atom type;
	int format;
	unsigned long n, after;
	unsigned char *p = NULL;
	if (XGetWindowProperty(win, w, prop, 0, 1, False, AnyPropertyType, &type, &format, &n, &after, &p) != Success ||
		p == NULL) {
		return 0;
	}
	int ok = n > 0 && format == 32;
	if (ok) {
		*out = *(unsigned long *)p;
	}
	XFree(p);
	return ok;
}

// lsActivePID возвращает PID процесса активного окна или 0.
int lsActivePID(void) {
	static Atom active, wmPID;
	if (active == None) {
		active = XInternAtom(win, "_NET_ACTIVE_WINDOW", False);
		wmPID = XInternAtom(win, "_NET_WM_PID", False);
	}
	unsigned long w, pid;
	if (!cardinalProp(DefaultRootWindow(win), active, &w) || w == 0 || !cardinalProp((Window)w, wmPID, &pid)) {
		return 0;
	}
	return (int)pid;
}
