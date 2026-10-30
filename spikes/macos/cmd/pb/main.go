//go:build darwin

// Command pb exercises the macOS pasteboard spike: it prints the pasteboard state, writes a lazy provider-backed item,
// writes a file promise and fulfils it, or polls the change count at the D-86 interval.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"time"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
	"github.com/kikz0r/edgesync/spikes/macos/pb"
)

const receiveTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pb:", err)
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "status", "status, provider, promise or watch")
	duration := flag.Duration("duration", 30*time.Second, "how long watch mode polls the change count")
	flag.Parse()

	lib, err := fw.Load()
	if err != nil {
		return err
	}
	board, err := pb.New(lib)
	if err != nil {
		return err
	}
	switch *mode {
	case "status":
		return status(board)
	case "provider":
		return provider(lib, board)
	case "promise":
		return promise(lib, board)
	case "watch":
		return watch(board, *duration)
	default:
		return fmt.Errorf("mode %q: want status, provider, promise or watch", *mode)
	}
}

func status(board *pb.Pasteboard) error {
	types, err := board.Types()
	if err != nil {
		return err
	}
	concealed, err := board.IsConcealed()
	if err != nil {
		return err
	}
	fmt.Printf("changeCount=%d concealed=%t\ntypes=%v\n", board.ChangeCount(), concealed, types)
	return nil
}

func provider(lib *fw.Lib, board *pb.Pasteboard) error {
	content, err := pb.SampleContent()
	if err != nil {
		return err
	}
	p, err := pb.NewProvider(lib, pb.ProviderConfig{ClassName: "ESSpikeProvider", Render: pb.RendererFor(content)})
	if err != nil {
		return err
	}
	defer p.Close()
	defer board.Clear()

	pbTypes := []string{pb.TypePlainText, pb.TypeHTML, pb.TypeRTF, pb.TypePNG}
	if err := p.Write(board, pbTypes...); err != nil {
		return err
	}
	announced, err := board.Types()
	if err != nil {
		return err
	}
	fmt.Printf("announced=%v\n", announced)
	for _, t := range pbTypes {
		fmt.Printf("after write: %s provider calls=%d\n", t, p.Calls(t))
	}
	for round := 1; round <= 2; round++ {
		for _, t := range pbTypes {
			data, err := board.Data(t)
			if err != nil {
				return err
			}
			fmt.Printf("read %d: %s bytes=%d provider calls=%d\n", round, t, len(data), p.Calls(t))
		}
	}
	board.Clear()
	fmt.Printf("after clearContents: pasteboardFinishedWithDataProvider calls=%d\n", p.Finished())
	return p.Err()
}

func promise(lib *fw.Lib, board *pb.Pasteboard) error {
	dir, err := os.MkdirTemp("", "edgesync-promise-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	pr, err := pb.NewPromise(lib, pb.PromiseConfig{
		ClassName: "ESSpikePromiseDelegate",
		FileType:  pb.PromiseFileType,
		FileName:  "promised.bin",
		Contents:  []byte("EdgeSync promised file"),
	})
	if err != nil {
		return err
	}
	defer pr.Close()
	defer board.Clear()

	path, err := pr.FulfillDirect(dir)
	if err != nil {
		return fmt.Errorf("direct fulfilment: %w", err)
	}
	fmt.Printf("direct fulfilment wrote %s\n", path)

	if err := pr.Write(board); err != nil {
		return err
	}
	types, err := board.Types()
	if err != nil {
		return err
	}
	fmt.Printf("types after writing the promise=%v\n", types)

	ctx, cancel := context.WithTimeout(context.Background(), receiveTimeout)
	defer cancel()
	received, err := pb.ReceivePromised(ctx, board, dir)
	switch {
	case err == nil:
		fmt.Printf("NSFilePromiseReceiver wrote %v (delegate name requests=%d)\n", received, pr.NameRequests())
	case errors.Is(err, pb.ErrNoPromise), errors.Is(err, context.DeadlineExceeded):
		fmt.Printf("OBSERVED: in-process NSFilePromiseReceiver path did not fulfil the promise: %v\n", err)
	default:
		return err
	}
	return pr.Err()
}

func watch(board *pb.Pasteboard, duration time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	fmt.Printf("polling every %s from changeCount=%d\n", pb.PollInterval, board.ChangeCount())
	var failure error
	err := board.Watch(ctx, pb.PollInterval, board.ChangeCount(), func(count int) {
		types, typesErr := board.Types()
		if typesErr != nil {
			failure = typesErr
			cancel()
			return
		}
		fmt.Printf("changeCount=%d concealed=%t types=%v\n", count, slices.Contains(types, pb.TypeConcealed), types)
	})
	if failure != nil {
		return failure
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
