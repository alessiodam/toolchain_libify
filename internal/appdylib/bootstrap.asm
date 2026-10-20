__libify_table_offset := {{.Offset}}
__libify_export_count := {{len .Exports}}

__libify_app_name:
	db "{{.App}}", 0
__libify_error_lut:
	dl __libify_err_missing
	dl __libify_err_format
	dl __libify_err_old
	dl __libify_err_init
	dl __libify_err_unset
	dl __libify_err_unknown
__libify_err_missing:
	db "library missing", 0
__libify_err_format:
	db "library format", 0
__libify_err_old:
	db "install newer library", 0
__libify_err_init:
	db "runtime-int failed", 0
__libify_err_unset:
	db "function unsupported, version", 0
__libify_err_unknown:
	db "unknown error", 0
__libify_errno:
	db 6
__libify_is_newer:
	db 0
__libify_max_errno := 6

{{.Prefix}}_is_newer:
	ld a, (__libify_is_newer)
	ret

{{.Prefix}}_get_start_errno:
	ld a, (__libify_errno)
	ret

{{.Prefix}}_get_start_errstring:
	ld a, (__libify_errno)
	or a, a
	jr z, .invalid
	dec a
	cp a, __libify_max_errno
	jr nc, .invalid
	ld l, a
	ld h, 3
	mlt hl
	ld de, __libify_error_lut
	add hl, de
	ld hl, (hl)
	ret
.invalid:
	ld hl, 0
	ret

{{.Prefix}}_start_with_crt:
	call ti._frameset0
	xor a, a
	ld (__libify_is_newer), a
	ld hl, (ix + 6)
	ld (_fn_imports_table + 0), hl
	ld hl, (ix + 9)
	ld (_fn_imports_table + 3), hl
	ld hl, (ix + 12)
	ld (_fn_imports_table + 6), hl

	push iy
	ld iy, ti.flags
	ld hl, __libify_app_name
	call ti.FindAppStart
	pop iy
	jp c, .app_missing

	push hl
	ld de, $112
	add hl, de
	ld hl, (hl)
	ld de, $100
	add hl, de
	pop de
	add hl, de
	ld de, __libify_table_offset
	add hl, de
{{range .Magic}}
	ld a, (hl)
	cp a, {{.}}
	jp nz, .table_fail
	inc hl
{{- end}}

	ld bc, (hl)
	inc hl
	inc hl
	inc hl

	push hl
	ld hl, __libify_export_count
	or a, a
	sbc hl, bc
	pop hl
	jr z, .count_ready
	jr nc, .count_ready
	ld bc, __libify_export_count
	ld a, 1
	ld (__libify_is_newer), a

.count_ready:
	push ix
	ld ix, __libify_jp_table + 1
.patch_loop:
	ld a, b
	or a, c
	jr z, .patch_done
	push hl
	ld hl, (hl)
{{- if .Retain}}
	ld de, 0
	or a, a
	sbc hl, de
	jr z, .patch_next
{{- end}}
	ld (ix + 0), hl
{{- if .Retain}}
.patch_next:
{{- end}}
	pop hl
	inc hl
	inc hl
	inc hl
	lea ix, ix + 4
	dec bc
	jr .patch_loop
.patch_done:
	pop ix

	ld hl, _fn_imports_table_end - _fn_imports_table
	push hl
	ld hl, _fn_imports_table
	push hl
	call {{.Init}}
	pop hl
	pop hl

	or a, a
	jr z, .init_failed
	pop ix
	ld a, 1
	ret

.init_failed:
	ld a, 4
	jr .fail
.table_fail:
	ld a, 2
	jr .fail
.app_missing:
	ld a, 1
.fail:
	ld (__libify_errno), a
	pop ix
	xor a, a
	ret

__libify_unset:
	ld a, 5
	ld (__libify_errno), a
	xor a, a
	ld hl, 0
	ld de, 0
	ld bc, 0
	ret
