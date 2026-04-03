# go-apispec — Call Graph Visualization

**License:** Apache License 2.0. See [LICENSE](../LICENSE) and [NOTICE](../NOTICE).

# Cytoscape.js Call Tree Diagram

This implementation provides a **tree-based call diagram** using Cytoscape.js that displays function call relationships in a hierarchical tree structure, similar to Mermaid but with better performance for large diagrams.

## Key Features

### 🎯 **Tree-Based Layout**
- **Hierarchical Structure**: Functions are arranged in a top-to-bottom tree layout
- **Fixed Positioning**: Nodes are positioned automatically and cannot be dragged
- **Rectangular Nodes**: Clean, professional rectangular nodes instead of circles
- **Clear Hierarchy**: Easy to trace function call chains from root to leaves

### 🎨 **Visual Design**
- **Rectangular Nodes**: Professional rectangular shape with rounded corners
- **Color Coding**: 
  - Blue: Regular functions
  - Red: Root functions (entry points)
  - Green: Function call edges
- **Clean Typography**: Readable labels with proper text wrapping
- **Modern UI**: Gradient background with clean controls

### 🚀 **Performance**
- **Optimized for Large Trees**: Handles thousands of nodes efficiently
- **Smooth Animations**: Fluid layout transitions
- **Fast Rendering**: Cytoscape.js provides excellent performance

### 🎮 **Interactive Features**
- **Click to Highlight**: Click any node to highlight its call chain
- **Zoom & Pan**: Navigate large trees easily
- **Layout Options**: Switch between different tree layouts
- **Export**: Save as PNG for documentation

## Controls

### Layout Options
- **Tree Layout (Dagre)**: Recommended hierarchical layout
- **Breadth-First Tree**: Alternative tree structure
- **Grid Layout**: Simple grid arrangement

### Interactive Controls
- **Reset**: Return to default view
- **Fit View**: Automatically fit all nodes to screen
- **Toggle Labels**: Show/hide node labels
- **Expand Tree**: Show all nodes
- **Collapse Tree**: Show only root and direct children
- **Export PNG**: Save diagram as image

### Keyboard Shortcuts
- `R`: Reset view
- `F`: Fit to view
- `L`: Toggle labels
- `E`: Expand tree
- `C`: Collapse tree

## Usage

The diagram is generated with the `--diagram` flag:

```bash
apispec --dir ./your-project --diagram diagram.html
```

This creates `diagram.html` which you can open in any web browser.

## Technical Details

### Layout Algorithm
- Uses **Dagre** layout engine for optimal tree positioning
- **Top-to-bottom** flow direction
- **Automatic spacing** between nodes and levels
- **Hierarchical ranking** for clear call chains

### Node Styling
- **Rectangular shape** with rounded corners
- **Fixed dimensions**: 120x50px for regular nodes
- **Text wrapping** for long function names
- **Border and shadow** effects for depth

### Edge Styling
- **Directed arrows** showing call direction
- **Curved lines** for better visual flow
- **Thick edges** (3px) for better visibility
- **Color-coded** for easy identification

## Advantages Over Mermaid

1. **Performance**: Much faster rendering for large diagrams
2. **Interactivity**: Click to highlight, zoom, pan
3. **Flexibility**: Multiple layout options
4. **Professional Look**: Modern UI with better styling
5. **Export Options**: Save as high-quality PNG
6. **Tree Structure**: Clear hierarchical layout
7. **Fixed Positioning**: No accidental node movement

## Browser Compatibility

Works in all modern browsers:
- Chrome/Chromium
- Firefox
- Safari
- Edge

## File Structure

```
go-apispec/
├── internal/spec/visualization.go                # Cytoscape generation logic
├── internal/spec/export.go                       # Export functions
├── internal/spec/paginated_export.go             # Paginated diagram support
├── internal/spec/templates/cytoscape_template.html   # Main diagram template
├── internal/spec/templates/paginated_template.html   # Paginated diagram template
├── internal/spec/templates/server_template.html      # Server-based diagram template
└── docs/CYTOGRAPHE_README.md                     # This documentation
```

The visualization code is in `internal/spec/visualization.go` and `export.go`. HTML templates are in `internal/spec/templates/` and embedded via `//go:embed`. Edges carry CFG branch context (if-then, if-else, switch-case) with color-coded rendering and case value labels.