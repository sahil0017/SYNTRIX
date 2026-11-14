package security

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

static OSStatus syntrix_keychain_set(const char *service, const char *account, const char *secret, int secret_len) {
    CFStringRef svc = CFStringCreateWithCString(kCFAllocatorDefault, service, kCFStringEncodingUTF8);
    CFStringRef acc = CFStringCreateWithCString(kCFAllocatorDefault, account, kCFStringEncodingUTF8);
    CFDataRef data = CFDataCreate(kCFAllocatorDefault, (const UInt8 *)secret, secret_len);

    const void *keys[] = { kSecClass, kSecAttrService, kSecAttrAccount };
    const void *vals[] = { kSecClassGenericPassword, svc, acc };
    CFDictionaryRef query = CFDictionaryCreate(kCFAllocatorDefault, keys, vals, 3,
                                               &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    SecItemDelete(query);

    const void *add_keys[] = { kSecClass, kSecAttrService, kSecAttrAccount, kSecValueData };
    const void *add_vals[] = { kSecClassGenericPassword, svc, acc, data };
    CFDictionaryRef add_dict = CFDictionaryCreate(kCFAllocatorDefault, add_keys, add_vals, 4,
                                                  &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    OSStatus status = SecItemAdd(add_dict, NULL);

    CFRelease(add_dict);
    CFRelease(query);
    CFRelease(data);
    CFRelease(acc);
    CFRelease(svc);
    return status;
}

static OSStatus syntrix_keychain_get(const char *service, const char *account, char **out_buf, int *out_len) {
    CFStringRef svc = CFStringCreateWithCString(kCFAllocatorDefault, service, kCFStringEncodingUTF8);
    CFStringRef acc = CFStringCreateWithCString(kCFAllocatorDefault, account, kCFStringEncodingUTF8);

    const void *keys[] = { kSecClass, kSecAttrService, kSecAttrAccount, kSecReturnData, kSecMatchLimit };
    const void *vals[] = { kSecClassGenericPassword, svc, acc, kCFBooleanTrue, kSecMatchLimitOne };
    CFDictionaryRef query = CFDictionaryCreate(kCFAllocatorDefault, keys, vals, 5,
                                               &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);

    CFTypeRef result = NULL;
    OSStatus status = SecItemCopyMatching(query, &result);
    if (status == errSecSuccess && result != NULL) {
        CFDataRef data = (CFDataRef)result;
        CFIndex len = CFDataGetLength(data);
        *out_buf = (char *)malloc((size_t)len + 1);
        if (*out_buf != NULL) {
            memcpy(*out_buf, CFDataGetBytePtr(data), (size_t)len);
            (*out_buf)[len] = '\0';
            *out_len = (int)len;
        } else {
            status = errSecAllocate;
        }
        CFRelease(result);
    }

    CFRelease(query);
    CFRelease(acc);
    CFRelease(svc);
    return status;
}

static OSStatus syntrix_keychain_delete(const char *service, const char *account) {
    CFStringRef svc = CFStringCreateWithCString(kCFAllocatorDefault, service, kCFStringEncodingUTF8);
    CFStringRef acc = CFStringCreateWithCString(kCFAllocatorDefault, account, kCFStringEncodingUTF8);

    const void *keys[] = { kSecClass, kSecAttrService, kSecAttrAccount };
    const void *vals[] = { kSecClassGenericPassword, svc, acc };
    CFDictionaryRef query = CFDictionaryCreate(kCFAllocatorDefault, keys, vals, 3,
                                               &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    OSStatus status = SecItemDelete(query);

    CFRelease(query);
    CFRelease(acc);
    CFRelease(svc);
    return status;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

const defaultService = "com.syntrix.desktop"

func SetKeychainSecret(account, secret string) error {
	cSvc := C.CString(defaultService)
	cAcc := C.CString(account)
	cSec := C.CString(secret)
	defer C.free(unsafe.Pointer(cSvc))
	defer C.free(unsafe.Pointer(cAcc))
	defer C.free(unsafe.Pointer(cSec))

	if st := C.syntrix_keychain_set(cSvc, cAcc, cSec, C.int(len(secret))); st != 0 {
		return fmt.Errorf("keychain set failed (OSStatus=%d)", int(st))
	}
	return nil
}

func GetKeychainSecret(account string) (string, error) {
	cSvc := C.CString(defaultService)
	cAcc := C.CString(account)
	defer C.free(unsafe.Pointer(cSvc))
	defer C.free(unsafe.Pointer(cAcc))

	var outBuf *C.char
	var outLen C.int
	if st := C.syntrix_keychain_get(cSvc, cAcc, &outBuf, &outLen); st != 0 {
		return "", fmt.Errorf("keychain get failed (OSStatus=%d)", int(st))
	}
	defer C.free(unsafe.Pointer(outBuf))
	return C.GoStringN(outBuf, outLen), nil
}

func DeleteKeychainSecret(account string) error {
	cSvc := C.CString(defaultService)
	cAcc := C.CString(account)
	defer C.free(unsafe.Pointer(cSvc))
	defer C.free(unsafe.Pointer(cAcc))

	if st := C.syntrix_keychain_delete(cSvc, cAcc); st != 0 && st != -25300 {
		return fmt.Errorf("keychain delete failed (OSStatus=%d)", int(st))
	}
	return nil
}
