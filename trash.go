package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const trashHelp = `CHUYỂN FILE VÀO THÙNG RÁC (không xoá vĩnh viễn)

  wj trash <file hoặc thư mục>...

Windows: Recycle Bin. macOS: ~/.Trash. Linux: thùng rác XDG (~/.local/share/Trash).
Khôi phục bằng trình quản lý file như mọi thứ khác trong thùng rác.
Không chuyển được (khác ổ đĩa, hết quyền) thì lệnh báo lỗi và để nguyên file: nó không
bao giờ xoá hẳn. Agent dùng lệnh này thay cho rm; wj doctor cũng dọn file tạm bằng nó.`

// moveToTrash is how wj removes anything that is not its own: to the trash,
// so a wrong guess can be undone. Tests replace it.
var moveToTrash = systemTrash

func runTrash(args []string, out io.Writer) error {
	if len(args) == 0 || contains([]string{"help", "-h", "-help", "--help"}, args[0]) {
		fmt.Fprintln(out, trashHelp)
		return nil
	}
	// Check everything first, so a typo moves nothing.
	for _, path := range args {
		if _, err := os.Lstat(path); err != nil {
			return fmt.Errorf("không thấy %s; chưa chuyển gì", path)
		}
	}
	if err := moveToTrash(args); err != nil {
		return err
	}
	var left []error
	for _, path := range args {
		if _, err := os.Lstat(path); err == nil {
			left = append(left, fmt.Errorf("%s vẫn còn đó", path))
			continue
		}
		fmt.Fprintln(out, "đã chuyển vào thùng rác:", path)
	}
	return errors.Join(left...)
}
