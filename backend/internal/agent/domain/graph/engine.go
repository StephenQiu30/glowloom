package graph

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

var (
	ErrGraphAlreadyCompiled = errors.New("graph is already compiled")
	ErrNodeNotFound         = errors.New("node not found")
	ErrDuplicateNode        = errors.New("node already exists")
	ErrNoEntryPoint         = errors.New("entry point not set")
	ErrMaxStepsExceeded     = errors.New("turn step budget exceeded")
	ErrCheckpointerRequired = errors.New("checkpointer required for resume")
	ErrNoCheckpointFound    = errors.New("no checkpoint found for run")
)

const (
	FinishPoint         = "__end__"
	DefaultMaxTurnSteps = 40
)

// NodeHandler processes the current state and returns an updated state or error.
type NodeHandler func(ctx context.Context, state *AgentState) (*AgentState, error)

// ConditionalEdgeRouter inspects the current state and returns the next node name.
type ConditionalEdgeRouter func(ctx context.Context, state *AgentState) (string, error)

// StateGraph defines the blueprint for a graph-based agent workflow.
type StateGraph struct {
	mu               sync.RWMutex
	nodes            map[string]NodeHandler
	edges            map[string]string
	conditionalEdges map[string]ConditionalEdgeRouter
	entryPoint       string
	finishPoint      string
	compiled         bool
}

// NewStateGraph creates a new uncompiled state graph.
func NewStateGraph() *StateGraph {
	return &StateGraph{
		nodes:            make(map[string]NodeHandler),
		edges:            make(map[string]string),
		conditionalEdges: make(map[string]ConditionalEdgeRouter),
		finishPoint:      FinishPoint,
	}
}

// AddNode registers a processing node.
func (g *StateGraph) AddNode(name string, handler NodeHandler) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.compiled {
		return ErrGraphAlreadyCompiled
	}
	if name == "" || name == FinishPoint {
		return fmt.Errorf("invalid node name: %q", name)
	}
	if _, exists := g.nodes[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateNode, name)
	}
	g.nodes[name] = handler
	return nil
}

// AddEdge registers a deterministic directed transition from one node to another.
func (g *StateGraph) AddEdge(from, to string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.compiled {
		return ErrGraphAlreadyCompiled
	}
	if from == "" || to == "" {
		return errors.New("edge endpoints cannot be empty")
	}
	g.edges[from] = to
	return nil
}

// AddConditionalEdge registers a dynamic routing function from a source node.
func (g *StateGraph) AddConditionalEdge(from string, router ConditionalEdgeRouter) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.compiled {
		return ErrGraphAlreadyCompiled
	}
	if from == "" || router == nil {
		return errors.New("conditional edge requires source node and router")
	}
	g.conditionalEdges[from] = router
	return nil
}

// SetEntryPoint defines the initial node of the graph.
func (g *StateGraph) SetEntryPoint(name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.compiled {
		return ErrGraphAlreadyCompiled
	}
	g.entryPoint = name
	return nil
}

// Compile validates graph topology and produces an executable CompiledGraph.
func (g *StateGraph) Compile(checkpointer Checkpointer) (*CompiledGraph, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.entryPoint == "" {
		return nil, ErrNoEntryPoint
	}
	if _, ok := g.nodes[g.entryPoint]; !ok {
		return nil, fmt.Errorf("%w (entry point): %s", ErrNodeNotFound, g.entryPoint)
	}
	g.compiled = true

	// Clone mappings to guarantee immutability after compilation
	nodes := make(map[string]NodeHandler, len(g.nodes))
	for k, v := range g.nodes {
		nodes[k] = v
	}
	edges := make(map[string]string, len(g.edges))
	for k, v := range g.edges {
		edges[k] = v
	}
	conditionalEdges := make(map[string]ConditionalEdgeRouter, len(g.conditionalEdges))
	for k, v := range g.conditionalEdges {
		conditionalEdges[k] = v
	}

	return &CompiledGraph{
		nodes:            nodes,
		edges:            edges,
		conditionalEdges: conditionalEdges,
		entryPoint:       g.entryPoint,
		checkpointer:     checkpointer,
	}, nil
}

// CompiledGraph is a compiled, thread-safe, executable state graph.
type CompiledGraph struct {
	nodes            map[string]NodeHandler
	edges            map[string]string
	conditionalEdges map[string]ConditionalEdgeRouter
	entryPoint       string
	checkpointer     Checkpointer
}

// Invoke starts execution of the graph from the configured entry point.
func (c *CompiledGraph) Invoke(ctx context.Context, initialState *AgentState) (*AgentState, error) {
	if initialState.MaxTurnSteps <= 0 {
		initialState.MaxTurnSteps = DefaultMaxTurnSteps
	}
	initialState.Status = "running"
	initialState.Interrupted = false
	initialState.InterruptReason = ""

	return c.invokeFromNode(ctx, initialState, c.entryPoint)
}

// Resume reloads the latest checkpoint for a run and continues execution from where it was suspended.
func (c *CompiledGraph) Resume(ctx context.Context, runID uuid.UUID, resumeData any) (*AgentState, error) {
	if c.checkpointer == nil {
		return nil, ErrCheckpointerRequired
	}
	cp, err := c.checkpointer.GetLatestCheckpoint(ctx, runID)
	if err != nil {
		return nil, err
	}
	if cp == nil || cp.State == nil {
		return nil, ErrNoCheckpointFound
	}

	state := cp.State
	state.Interrupted = false
	state.InterruptReason = ""
	state.Status = "running"

	// Determine next node from the suspended node's outgoing edge
	currentNode := state.CurrentNode
	var nextNode string
	if router, ok := c.conditionalEdges[currentNode]; ok {
		next, err := router(ctx, state)
		if err != nil {
			return state, err
		}
		nextNode = next
	} else if next, ok := c.edges[currentNode]; ok {
		nextNode = next
	} else {
		nextNode = FinishPoint
	}

	return c.invokeFromNode(ctx, state, nextNode)
}

func (c *CompiledGraph) invokeFromNode(ctx context.Context, state *AgentState, startNode string) (*AgentState, error) {
	currentNode := startNode
	for currentNode != FinishPoint {
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		default:
		}

		// Turn budget watchdog check
		if state.TurnStepCount >= state.MaxTurnSteps {
			state.Status = "failed"
			state.InterruptReason = "turn_step_limit_exceeded"
			return state, ErrMaxStepsExceeded
		}
		state.TurnStepCount++

		handler, ok := c.nodes[currentNode]
		if !ok {
			return state, fmt.Errorf("%w: %s", ErrNodeNotFound, currentNode)
		}

		state.CurrentNode = currentNode
		nextState, err := handler(ctx, state)
		if err != nil {
			return state, err
		}
		state = nextState

		// Save checkpoint after node execution
		if c.checkpointer != nil {
			cpID := uuid.New()
			state.CheckpointID = cpID
			if err := c.checkpointer.SaveCheckpoint(ctx, state.RunID, cpID, currentNode, state); err != nil {
				return state, fmt.Errorf("save checkpoint: %w", err)
			}
		}

		// Proposal Yield / Human-in-the-Loop Interrupt
		if state.Interrupted {
			state.Status = "suspended_for_approval"
			return state, nil
		}

		// Route to next node
		if router, hasRouter := c.conditionalEdges[currentNode]; hasRouter {
			next, err := router(ctx, state)
			if err != nil {
				return state, err
			}
			currentNode = next
		} else if next, hasEdge := c.edges[currentNode]; hasEdge {
			currentNode = next
		} else {
			currentNode = FinishPoint
		}
	}

	state.Status = "completed"
	return state, nil
}
