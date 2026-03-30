/*
 * Shim for __wrap_log_printf — needed when using custom_sdkconfig with pioarduino.
 *
 * The pre-compiled Arduino framework uses -Wl,--wrap=log_printf, which redirects
 * all log_printf() calls to __wrap_log_printf(). In normal builds, this symbol is
 * provided by the pre-compiled libespressif__esp_diagnostics.a. When custom_sdkconfig
 * triggers a source rebuild, that component is recompiled without the wrapper,
 * causing "undefined reference to __wrap_log_printf" linker errors.
 *
 * This shim provides __wrap_log_printf by forwarding to log_printfv (the real
 * implementation in the Arduino core's esp32-hal-uart.c).
 */

#include <stdarg.h>

/* Defined in Arduino core: esp32-hal-uart.c */
extern int log_printfv(const char *format, va_list arg);

int __wrap_log_printf(const char *format, ...) {
    va_list args;
    va_start(args, format);
    int ret = log_printfv(format, args);
    va_end(args);
    return ret;
}
