package cmd

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/spf13/cobra"
)

// The `plan image` subcommand group keeps the images a plan draws — a design
// mock-up of a visual change, a flow chart of a complex flow — beside the
// plan's documents, each addressed by the feature's bare name and an image
// name. Images are stored byte for byte: unlike a plan document they carry no
// lifecycle front matter, which would stop them rendering. The only format
// accepted is SVG, so an agent can draw one as text and a Markdown viewer
// renders it from the relative link a write reports.
func init() {
	imageCmd := &cobra.Command{Use: "image", Short: "Write and read the SVG images a plan links to", RunE: runUnknownSubcommand}

	var fromPath string
	write := &cobra.Command{
		Use:   "write <feature> <image>",
		Short: "Write an SVG source file into the plan store as one of a plan's images",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			feature, image, err := parseImageAddress(cmd, args)
			if err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if err := validateIDPrefix(cfg, feature); err != nil {
				return err
			}
			content, err := os.ReadFile(fromPath)
			if err != nil {
				return fmt.Errorf("reading source file %q: %w", fromPath, err)
			}
			if err := validateSVG(content); err != nil {
				return output.NewError("image_not_svg",
					fmt.Sprintf("%q is not an SVG image: %v", fromPath, err)).
					WithResource(feature + " " + image).
					WithNextAction("draw the image as a well-formed SVG document whose root element is <svg>, then write it again")
			}
			st, storeDir, err := storeFileStore(planStoreDir)
			if err != nil {
				return err
			}
			storePath := artifact.ImageStorePath(storeDir, feature, image)
			if err := st.Write(storePath, content); err != nil {
				return err
			}
			return output.Write(cmd.OutOrStdout(), imageEntry(storeDir, feature, image), "")
		},
	}
	write.Flags().StringVar(&fromPath, "from", "", "Path to the SVG source file whose contents will be written into the store")
	_ = write.MarkFlagRequired("from")

	read := &cobra.Command{
		Use:   "read <feature> <image>",
		Short: "Read one of a plan's images and write it to stdout",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			feature, image, err := parseImageAddress(cmd, args)
			if err != nil {
				return err
			}
			st, storeDir, err := storeFileStore(planStoreDir)
			if err != nil {
				return err
			}
			content, err := st.Read(artifact.ImageStorePath(storeDir, feature, image))
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return imageNotFound(feature, image)
				}
				return err
			}
			_, err = cmd.OutOrStdout().Write(content)
			return err
		},
	}

	del := &cobra.Command{
		Use:   "delete <feature> <image>",
		Short: "Delete one of a plan's images from the store",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			feature, image, err := parseImageAddress(cmd, args)
			if err != nil {
				return err
			}
			st, storeDir, err := storeFileStore(planStoreDir)
			if err != nil {
				return err
			}
			return st.Delete(artifact.ImageStorePath(storeDir, feature, image))
		},
	}

	list := &cobra.Command{
		Use:   "list <feature>",
		Short: "List a plan's images by name, with the link its documents use",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			feature, err := artifact.ParseFeature(artifact.KindPlan, args[0])
			if err != nil {
				return addressRefusal(cmd, artifact.KindPlan, args, err)
			}
			st, storeDir, err := storeFileStore(planStoreDir)
			if err != nil {
				return err
			}
			if !st.Exists(artifact.FeatureDir(storeDir, feature)) {
				return addressNotFound(cmd, artifact.Address{Kind: artifact.KindPlan, Feature: feature})
			}
			images := make([]map[string]any, 0)
			entries, err := st.List(artifact.ImagesDir(storeDir, feature))
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			for _, e := range entries {
				name, ok := artifact.ImageNameFromEntry(e.Name, e.IsDir)
				if !ok {
					continue
				}
				images = append(images, imageEntry(storeDir, feature, name))
			}
			return output.Write(cmd.OutOrStdout(), map[string]any{"images": images}, "")
		},
	}

	imageCmd.AddCommand(write, read, del, list)
	planCmd.AddCommand(imageCmd)
}

// planStoreDir selects the configured plan directory.
func planStoreDir(c config.Config) string { return c.Plan.Config.Directory }

// imageEntry describes one stored image: its name, where the store keeps it,
// and the relative link a plan document uses to show it.
func imageEntry(storeDir, feature, image string) map[string]any {
	return map[string]any{
		"name": image,
		"path": reportedLocation(centralLocationBase, artifact.ImageStorePath(storeDir, feature, image)),
		"link": artifact.ImageLink(image),
	}
}

// parseImageAddress validates the <feature> <image> arguments of an image
// verb. A segment carrying an extension or a path is refused with the
// caller's command correctly spelled, before any store is touched.
func parseImageAddress(cmd *cobra.Command, args []string) (feature, image string, err error) {
	corrected := make([]string, len(args))
	bad := ""
	for i, a := range args {
		seg, perr := artifact.ParseFeature(artifact.KindPlan, a)
		var extErr *artifact.ExtensionError
		switch {
		case perr == nil:
			corrected[i] = seg
		case errors.As(perr, &extErr):
			corrected[i] = extErr.Corrected.Feature
			if bad == "" {
				bad = a
			}
		default:
			return "", "", output.NewError("bad_input", fmt.Sprintf("invalid plan image address %q: %v", a, perr)).
				WithNextAction(fmt.Sprintf("run `%s plan image list <feature>` to see the names it accepts", storeCommandName()))
		}
	}
	if bad != "" {
		return "", "", output.NewError(artifact.ErrCodeUnexpectedExtension,
			fmt.Sprintf("%q carries a file extension or path; plan images are addressed as <feature> <image>, two arguments with no extension", bad)).
			WithResource(bad).
			WithNextAction(fmt.Sprintf("run `%s`", restateCommand(cmd, corrected)))
	}
	return corrected[0], corrected[1], nil
}

// imageNotFound refuses a correctly spelled address that names no stored
// image, pointing at the list of the plan's images.
func imageNotFound(feature, image string) error {
	return output.NewError("not_found", fmt.Sprintf("plan %q has no image named %q", feature, image)).
		WithResource(feature + " " + image).
		WithNextAction(fmt.Sprintf("run `%s plan image list %s` to see available names", storeCommandName(), feature))
}

// validateSVG accepts a well-formed XML document whose root element is <svg>.
// It is the whole of the format check: what the drawing shows is the plan
// author's business, not the store's.
func validateSVG(content []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(content))
	root := ""
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("not well-formed XML: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok && root == "" {
			root = start.Name.Local
			if root != "svg" {
				return fmt.Errorf("the root element is <%s>, not <svg>", root)
			}
		}
	}
	if root == "" {
		return errors.New("it holds no XML element")
	}
	return nil
}
