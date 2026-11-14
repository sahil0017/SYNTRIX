package research

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework PDFKit -framework Foundation
#import <Foundation/Foundation.h>
#import <PDFKit/PDFKit.h>
#include <stdlib.h>
#include <string.h>

static char* syntrix_extract_pdf_text(const char *path) {
    @autoreleasepool {
        NSString *nsPath = [NSString stringWithUTF8String:path];
        NSURL *url = [NSURL fileURLWithPath:nsPath];
        PDFDocument *doc = [[PDFDocument alloc] initWithURL:url];
        if (!doc) {
            return NULL;
        }
        NSMutableString *fullText = [NSMutableString string];
        NSUInteger count = [doc pageCount];
        for (NSUInteger i = 0; i < count; i++) {
            PDFPage *page = [doc pageAtIndex:i];
            NSString *pageText = [page string];
            if (pageText) {
                [fullText appendString:pageText];
                [fullText appendString:@"\n"];
            }
        }
        const char *utf8 = [fullText UTF8String];
        if (!utf8) {
            return NULL;
        }
        return strdup(utf8);
    }
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func ExtractPDFText(path string) (string, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	cText := C.syntrix_extract_pdf_text(cPath)
	if cText == nil {
		return "", fmt.Errorf("failed to open or parse PDF: %s", path)
	}
	defer C.free(unsafe.Pointer(cText))
	return C.GoString(cText), nil
}
