// SPDX-License-Identifier: Unlicense OR MIT

// Package headless implements headless windows for rendering
// an operation list to an image.
package headless

import (
	"errors"
	"image"
	"image/color"
	"runtime"

	"gioui.org/gpu"
	"gioui.org/gpu/internal/driver"
	"gioui.org/op"
)

// Window is a headless window.
type Window struct {
	size   image.Point
	ctx    context
	dev    driver.Device
	gpu    gpu.GPU
	fboTex driver.Texture
}

type context interface {
	API() gpu.API
	MakeCurrent() error
	ReleaseCurrent()
	Release()
}

var (
	newContextPrimary  func() (context, error)
	newContextFallback func() (context, error)
)

func newContext() (context, error) {
	funcs := []func() (context, error){newContextPrimary, newContextFallback}
	var firstErr error
	for _, f := range funcs {
		if f == nil {
			continue
		}
		c, err := f()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return c, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, errors.New("x11: no available GPU backends")
}

// NewWindow creates a new headless window.
func NewWindow(width, height int) (*Window, error) {
	ctx, err := newContext()
	if err != nil {
		return nil, err
	}
	w := &Window{
		size: image.Point{X: width, Y: height},
		ctx:  ctx,
	}
	err = contextDo(ctx, func() error {
		api := ctx.API()
		dev, err := driver.NewDevice(api)
		if err != nil {
			return err
		}
		fboTex, err := dev.NewTexture(
			driver.TextureFormatSRGBA,
			width, height,
			driver.FilterNearest, driver.FilterNearest,
			driver.BufferBindingFramebuffer,
		)
		if err != nil {
			return nil
		}
		gp, err := gpu.New(api)
		if err != nil {
			fboTex.Release()
			dev.Release()
			return err
		}
		w.fboTex = fboTex
		w.gpu = gp
		w.dev = dev
		return err
	})
	if err != nil {
		ctx.Release()
		return nil, err
	}
	return w, nil
}

// Release resources associated with the window.
func (w *Window) Release() {
	contextDo(w.ctx, func() error {
		if w.fboTex != nil {
			w.fboTex.Release()
			w.fboTex = nil
		}
		if w.gpu != nil {
			w.gpu.Release()
			w.gpu = nil
		}
		if w.dev != nil {
			w.dev.Release()
			w.dev = nil
		}
		return nil
	})
	if w.ctx != nil {
		w.ctx.Release()
		w.ctx = nil
	}
}

// Frame replace the window content and state with the
// operation list.
func (w *Window) Frame(frame *op.Ops) error {
	return contextDo(w.ctx, func() error {
		w.gpu.Clear(color.NRGBA{})
		return w.gpu.Frame(frame, w.fboTex, w.size)
	})
}

// Screenshot returns an image with the content of the window.
func (w *Window) Screenshot() (*image.RGBA, error) {
	var img *image.RGBA
	err := contextDo(w.ctx, func() error {
		var err error
		img, err = driver.DownloadImage(w.dev, w.fboTex, image.Rectangle{Max: w.size})
		return err
	})
	if err != nil {
		return nil, err
	}
	return img, nil
}

func contextDo(ctx context, f func() error) error {
	errCh := make(chan error)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ctx.MakeCurrent(); err != nil {
			errCh <- err
			return
		}
		err := f()
		ctx.ReleaseCurrent()
		errCh <- err
	}()
	return <-errCh
}
