#include <stdbool.h>
#include <stddef.h>
#include <string.h>

#include "../demoapp.h"

extern char __bss_low[];
extern size_t __bss_len[];
extern char __data_vma[];
extern char __data_lma[];
extern size_t __data_len[];

static uint24_t calls = 1;

bool dapp_init(const void *imports, size_t length)
{
    (void)imports;
    (void)length;
    memset(__bss_low, 0, (size_t)__bss_len);
    memcpy(__data_vma, __data_lma, (size_t)__data_len);
    return true;
}

uint24_t dapp_counter(void)
{
    return calls++;
}

int main(void)
{
    return 0;
}
