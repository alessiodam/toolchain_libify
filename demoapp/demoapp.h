#ifndef DEMOAPP_H
#define DEMOAPP_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

bool dapp_start_with_crt(void *malloc_fn, void *free_fn, void *realloc_fn);
uint8_t dapp_get_start_errno(void);
const char *dapp_get_start_errstring(void);
uint24_t dapp_counter(void);

#endif
