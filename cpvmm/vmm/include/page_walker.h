/*
 * Copyright (c) 2013 Intel Corporation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *     http://www.apache.org/licenses/LICENSE-2.0
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#ifndef PAGE_WALKER_H

#include <vmm_defs.h>
#include <guest_cpu.h>

typedef enum {
    PW_RETVAL_SUCCESS,
    PW_RETVAL_PF,
    PW_RETVAL_PHYS_MEM_VIOLATION,
    PW_RETVAL_FAIL,
} PW_RETVAL;

#define PW_INVALID_GPA (~((UINT64)0))
#define PW_NUM_OF_PDPT_ENTRIES_IN_32_BIT_MODE 4
#define PW_SIZE_OF_PAE_ENTRY 8

/* Function: pw_perform_page_walk
 * Description: The function performes page walk over guest page tables
 *              for specific virtual address
 * Input:
 *       gcpu - gcpu handle
 *       virt_addr - virtual address to perform page walk for
 *       is_write - indicates whether it is write access
 *       is_user - indicates whether it is a user access
 *       is_fetch - indicates whether it is a fetch access
 *       set_ad_bits - if TRUE, A/D bits will be set in guest table
 * Output:
 *       gpa - final guest physical addres, see "Ret. value" description
 *             for detailed information about this value.
 *       pfec - page fault error code in case when page walk will return "PW_RETVAL_PF".
 * Ret. value:
 *       PW_RETVAL_SUCCESS - the page walk succeeded, "gpa" output variable
 *                           contains the final physical address, "pfec" output variable contains "garbage".
 *       PW_RETVAL_PF - the page fault exception should occur, "pfec" output variable contains
 *                      error code, "gpa" output variable contains "PW_INVALID_GPA" value in case
 *                      when page walk could not calculate the final address. In case when this is
 *                      a "protection fault" (the permissions are inconsistent), "gpa" variable will
 *                      contain the target guest physical address.
 *       PW_RETVAL_PHYS_MEM_VIOLATION - the page walker could not retrieve "host physical address" for
 *                                      inner tables and thus could not retrieve the pointer and could not
 *                                      read the content of the entries. In this case "gpa" variable will
 *                                      contain "PW_INVALID_GPA" and "pfec" will contain "garbage".
 *       PW_RETVAL_FAIL - some internal error has occurred, must assert.
 * Note:
 *       If access attributes (WRITE, USER, FETCH) are not important, use "FALSE" value for "is_write",
 *       "is_user" and "is_fetch" varables.
 */
PW_RETVAL pw_perform_page_walk(IN GUEST_CPU_HANDLE gcpu,
                 IN UINT64 virt_addr, IN BOOLEAN is_write,
                 IN BOOLEAN is_user, IN BOOLEAN is_fetch,
                 IN BOOLEAN set_ad_bits, OUT UINT64* gpa, OUT UINT64* pfec);

/* Function: pw_is_pdpt_in_32_bit_pae_mode_valid
 * Description: The function performes page walk over guest page tables
 *              for specific virtual address
 * Input:
 *       gcpu - gcpu handle
 *       pdpt_ptr - pointer (HVA) to PDPT
 * Ret.value: TRUE in case no reserved bits are set in PDPT, FALSE otherwise
 */
BOOLEAN pw_is_pdpt_in_32_bit_pae_mode_valid(IN GUEST_CPU_HANDLE gcpu,
                                            IN void* pdpt_ptr);

#endif
