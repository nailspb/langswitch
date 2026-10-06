#import <AppKit/AppKit.h>
#include <libproc.h>
#include <string.h>

// lsActiveApp записывает в buf имя исполняемого файла активного приложения.
// frontmostApplication обновляется главным циклом приложения, читать его можно из любого потока.
int lsActiveApp(char *buf, int n) {
	@autoreleasepool {
		pid_t pid = NSWorkspace.sharedWorkspace.frontmostApplication.processIdentifier;
		char path[PROC_PIDPATHINFO_MAXSIZE];
		if (pid <= 0 || proc_pidpath(pid, path, sizeof(path)) <= 0) {
			return 0;
		}
		const char *name = strrchr(path, '/');
		strlcpy(buf, name != NULL ? name + 1 : path, n);
		return 1;
	}
}
