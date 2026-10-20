// Package filepicker is a file picker made of gift views: it opens one or
// several files, chooses a directory or picks a path to save to, inside the
// application's own window and on a touchscreen as well as with a mouse.
//
// It reads through an [io/fs.FS], so an application decides what the person
// may see — os.DirFS("/media/usb") on a kiosk — and a test hands it a
// [testing/fstest.MapFS]. It never writes.
package filepicker
