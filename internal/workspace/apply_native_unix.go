//go:build darwin || linux

package workspace

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type applyUnixMetadata struct {
	UID, GID uint32
	RawMode  uint32
}

func applyOpenDirectory(path string) (*os.File, error) {
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("%w An application directory must have an absolute path.", ErrApplyUnsupported)
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	current := os.NewFile(uintptr(fd), "/")
	for _, name := range strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/") {
		if name == "" {
			continue
		}
		// Untouched ancestors may legitimately be sticky (for example /tmp).
		// Only no-follow directory traversal is required here; the returned
		// application parent is separately checked by applyNativeInfo.
		nextFD, openErr := unix.Openat(int(current.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_NOCTTY, 0)
		var next *os.File
		if openErr == nil {
			next = os.NewFile(uintptr(nextFD), name)
		}
		closeErr := current.Close()
		if openErr != nil {
			return nil, errors.Join(openErr, closeErr)
		}
		if closeErr != nil {
			return nil, errors.Join(closeErr, next.Close())
		}
		current = next
	}
	return current, nil
}

func applyLeafName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\x00")
}

func applyOpenRelative(parent *os.File, name string, create, directory bool) (*os.File, error) {
	if parent == nil || !applyLeafName(name) || create && directory {
		return nil, fmt.Errorf("%w An application entry must be one ordinary child name.", ErrApplyUnsupported)
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_NOCTTY
	if directory {
		flags |= unix.O_DIRECTORY
	}
	if create {
		flags = flags&^unix.O_ACCMODE | unix.O_RDWR | unix.O_CREAT | unix.O_EXCL
	}
	fd, err := unix.Openat(int(parent.Fd()), name, flags, 0600)
	if err != nil {
		if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
			return nil, fmt.Errorf("%w The application entry is a redirect or non-directory: %v", ErrApplyUnsupported, err)
		}
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err == nil {
		err = applyCheckUnixType(st, directory)
	}
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

func applyCheckUnixType(st unix.Stat_t, directory bool) error {
	typeBits := uint32(st.Mode) & unix.S_IFMT
	if directory && typeBits != unix.S_IFDIR || !directory && (typeBits != unix.S_IFREG || st.Nlink != 1) {
		return fmt.Errorf("%w Application entries must be ordinary directories or single-link regular files.", ErrApplyUnsupported)
	}
	// A directory's setgid bit supplies the explicit new-file group rule. Other
	// special bits, and all special bits on regular files, cannot be reproduced.
	allowed := uint32(0)
	if directory {
		allowed = unix.S_ISGID
	}
	if uint32(st.Mode)&07000&^allowed != 0 {
		return fmt.Errorf("%w The application entry has unsupported special mode bits.", ErrApplyUnsupported)
	}
	return nil
}

func applyStatUnchanged(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Nlink == b.Nlink &&
		a.Uid == b.Uid && a.Gid == b.Gid && a.Size == b.Size && a.Ctim == b.Ctim && a.Mtim == b.Mtim
}

func applyNativeInfo(file *os.File, directory bool) (applyFileInfo, error) {
	if file == nil {
		return applyFileInfo{}, fmt.Errorf("%w The application descriptor is missing.", ErrApplyUnsupported)
	}
	var before, after unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &before); err != nil {
		return applyFileInfo{}, err
	}
	if err := applyCheckUnixType(before, directory); err != nil {
		return applyFileInfo{}, err
	}
	volume, err := applyUnixPlatformMetadata(file, before, directory)
	if err != nil {
		return applyFileInfo{}, err
	}
	if err = unix.Fstat(int(file.Fd()), &after); err != nil {
		return applyFileInfo{}, err
	}
	if !applyStatUnchanged(before, after) {
		return applyFileInfo{}, fmt.Errorf("%w Application metadata changed during inspection.", ErrApplyConflict)
	}
	return applyFileInfo{
		Identity: fmt.Sprintf("unix:%x:%x", uint64(before.Dev), uint64(before.Ino)),
		Volume:   volume, Owner: strconv.FormatUint(uint64(before.Uid), 10), Group: strconv.FormatUint(uint64(before.Gid), 10),
		Mode: uint32(before.Mode) & 0777, Control: uint32(before.Mode) & 07000,
		Native: applyUnixMetadata{UID: before.Uid, GID: before.Gid, RawMode: uint32(before.Mode)},
	}, nil
}

func applyPrepareMetadata(stage *os.File, expected applyFileInfo) error {
	return applyRestoreMetadata(stage, expected)
}

// Only the common application controller's exclusively created private staging
// handle may be passed here. Destination and quarantined originals are read-only.
func applyRestoreMetadata(stage *os.File, expected applyFileInfo) error {
	metadata, ok := expected.Native.(applyUnixMetadata)
	if !ok || metadata.RawMode&unix.S_IFMT != unix.S_IFREG || expected.Mode&^0777 != 0 ||
		expected.Control != 0 || expected.Security != "" ||
		expected.Owner != strconv.FormatUint(uint64(metadata.UID), 10) || expected.Group != strconv.FormatUint(uint64(metadata.GID), 10) {
		return fmt.Errorf("%w The intended staged metadata is invalid.", ErrApplyUnsupported)
	}
	before, err := applyNativeInfo(stage, false)
	if err != nil {
		return err
	}
	flags, err := unix.FcntlInt(stage.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDWR {
		return fmt.Errorf("%w Metadata requires the private writable staging handle: %v", ErrApplyUnsupported, err)
	}
	if before.Owner != expected.Owner || before.Group != expected.Group {
		if err = unix.Fchown(int(stage.Fd()), int(metadata.UID), int(metadata.GID)); err != nil {
			return fmt.Errorf("%w The staged owner or group cannot be preserved: %v", ErrApplyUnsupported, err)
		}
	}
	if err = unix.Fchmod(int(stage.Fd()), expected.Mode); err != nil {
		return fmt.Errorf("%w The staged mode cannot be preserved: %v", ErrApplyUnsupported, err)
	}
	after, err := applyNativeInfo(stage, false)
	if err != nil {
		return err
	}
	if after.Identity != before.Identity || after.Volume != before.Volume || after.Owner != expected.Owner ||
		after.Group != expected.Group || after.Mode != expected.Mode || after.Control != expected.Control || after.Security != expected.Security {
		return fmt.Errorf("%w Staged metadata did not match its required readback.", ErrApplyUnsupported)
	}
	return nil
}

func applyNewFileMetadata(parent *os.File, mode uint32) (applyFileInfo, error) {
	if mode&^0777 != 0 {
		return applyFileInfo{}, fmt.Errorf("%w A new application file requires a basic mode.", ErrApplyUnsupported)
	}
	info, err := applyNativeInfo(parent, true)
	if err != nil {
		return applyFileInfo{}, err
	}
	parentMetadata := info.Native.(applyUnixMetadata)
	uid := uint32(os.Geteuid())
	gid, err := applyUnixNewFileGroup(parentMetadata)
	if err != nil {
		return applyFileInfo{}, err
	}
	return applyFileInfo{
		Volume: info.Volume, Owner: strconv.FormatUint(uint64(uid), 10), Group: strconv.FormatUint(uint64(gid), 10), Mode: mode,
		Native: applyUnixMetadata{UID: uid, GID: gid, RawMode: unix.S_IFREG | mode},
	}, nil
}

func applyRenameExclusive(sourceParent *os.File, sourceName string, source *os.File, destinationParent *os.File, destinationName string) error {
	if sourceParent == nil || source == nil || destinationParent == nil || !applyLeafName(sourceName) || !applyLeafName(destinationName) {
		return fmt.Errorf("%w An exclusive move requires retained parents and child names.", ErrApplyUnsupported)
	}
	sourceInfo, err := applyNativeInfo(source, false)
	if err != nil {
		return err
	}
	for _, parent := range []*os.File{sourceParent, destinationParent} {
		info, inspectErr := applyNativeInfo(parent, true)
		if inspectErr != nil {
			return inspectErr
		}
		if info.Volume != sourceInfo.Volume {
			return fmt.Errorf("%w An application move crosses mounted filesystems.", ErrApplyUnsupported)
		}
	}
	var opened, named unix.Stat_t
	if err = unix.Fstat(int(source.Fd()), &opened); err != nil {
		return err
	}
	if err = unix.Fstatat(int(sourceParent.Fd()), sourceName, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("%w The source name is no longer available: %v", ErrApplyConflict, err)
	}
	if !applyStatUnchanged(opened, named) {
		return fmt.Errorf("%w The source name no longer identifies the retained object.", ErrApplyConflict)
	}
	// Unix rename is relative to retained directories, but not to the source
	// descriptor. The common controller must verify the displaced/published
	// object afterward and retain any last-moment competing object as partial.
	if err = applyUnixRenameExclusive(int(sourceParent.Fd()), sourceName, int(destinationParent.Fd()), destinationName); err != nil {
		if errors.Is(err, unix.EEXIST) || errors.Is(err, unix.ENOTEMPTY) || errors.Is(err, unix.ENOENT) {
			return fmt.Errorf("%w The exclusive move encountered a changed name: %v", ErrApplyConflict, err)
		}
		if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
			return fmt.Errorf("%w The filesystem cannot perform this anchored exclusive move: %v", ErrApplyUnsupported, err)
		}
		return err
	}
	return nil
}

// A namespace observation does not mutate either object and does not follow
// symlinks. The retained descriptor keeps the checked inode live across rename.
func applyMatchesName(parent *os.File, name string, file *os.File) (bool, error) {
	if parent == nil || file == nil || !applyLeafName(name) {
		return false, ErrApplyUnsupported
	}
	var retained, named unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &retained); err != nil {
		return false, err
	}
	if err := unix.Fstatat(int(parent.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return false, nil
		}
		return false, err
	}
	if err := applyCheckUnixType(retained, false); err != nil {
		return false, err
	}
	if err := applyCheckUnixType(named, false); err != nil {
		return false, err
	}
	return retained.Dev == named.Dev && retained.Ino == named.Ino, nil
}

func applySyncDirectory(directory *os.File) error {
	if _, err := applyNativeInfo(directory, true); err != nil {
		return err
	}
	if err := unix.Fsync(int(directory.Fd())); err != nil {
		return fmt.Errorf("%w Application directory durability is unavailable: %v", ErrApplyUnsupported, err)
	}
	return nil
}

// The single-attribute Darwin call deliberately excludes RETURNED_ATTRS and
// PACK_INVAL_ATTRS: an unsupported requested attribute must fail, not become a
// default. This establishes only the OS API's reported ACL state. XNU can hide
// corrupt filesystem fallback records; this parser makes no forensic claim.
func applyParseDarwinACL(data []byte) error {
	invalid := func(reason string) error {
		return fmt.Errorf("%w Darwin ACL metadata is %s.", ErrApplyUnsupported, reason)
	}
	if len(data) < 12 {
		return invalid("incomplete")
	}
	total := uint64(binary.LittleEndian.Uint32(data[:4]))
	offset := int32(binary.LittleEndian.Uint32(data[4:8]))
	length := uint64(binary.LittleEndian.Uint32(data[8:12]))
	if total < 12 || total > uint64(len(data)) || total > 8192 || total%4 != 0 || offset != 8 || 12+length != total {
		return invalid("malformed or truncated")
	}
	if length == 0 {
		return nil
	}
	if length < 44 {
		return invalid("incomplete")
	}
	security := data[12:total]
	if binary.LittleEndian.Uint32(security[:4]) != 0x012cc16d {
		return invalid("unrecognized")
	}
	for _, value := range security[4:36] {
		if value != 0 {
			return invalid("unrecognized")
		}
	}
	if binary.LittleEndian.Uint32(security[40:44]) != 0 {
		return invalid("unpreservable")
	}
	count := binary.LittleEndian.Uint32(security[36:40])
	if count == ^uint32(0) && length == 44 {
		return nil
	}
	if count > 128 || length != 44+24*uint64(count) {
		return invalid("malformed")
	}
	return invalid("an unsupported empty or populated ACL")
}
