package repository

import (
	"context"
	"fmt"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Neo4jRepository struct {
	driver neo4j.DriverWithContext
}

// GraphNode merepresentasikan satu simpul dalam Knowledge Graph
type GraphNode struct {
	Label      string
	Properties map[string]interface{}
}

// GraphEdge merepresentasikan relasi/garis antara dua simpul
type GraphEdge struct {
	Type         string
	SourceNode   GraphNode
	TargetNode   GraphNode
	Properties   map[string]interface{}
}

func NewNeo4jRepository(uri, username, password string) (*Neo4jRepository, error) {
	if uri == "" || username == "" || password == "" {
		return nil, fmt.Errorf("neo4j credentials empty")
	}

	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	err = driver.VerifyConnectivity(ctx)
	if err != nil {
		return nil, err
	}

	return &Neo4jRepository{driver: driver}, nil
}

func (r *Neo4jRepository) Close(ctx context.Context) error {
	if r.driver != nil {
		return r.driver.Close(ctx)
	}
	return nil
}

func (r *Neo4jRepository) SaveKnowledgeGraph(ctx context.Context, nodes []GraphNode, edges []GraphEdge) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// 1. MERGE Nodes
		for _, n := range nodes {
			idVal, ok := n.Properties["id"]
			if !ok {
				continue
			}

			// Hindari Cypher Injection: Pastikan n.Label valid huruf
			cypher := fmt.Sprintf(`MERGE (n:%s {id: $id}) SET n += $props`, n.Label)
			
			params := map[string]interface{}{
				"id":    idVal,
				"props": n.Properties,
			}
			_, err := tx.Run(ctx, cypher, params)
			if err != nil {
				return nil, err
			}
		}

		// 2. MERGE Edges
		for _, e := range edges {
			sID, okS := e.SourceNode.Properties["id"]
			tID, okT := e.TargetNode.Properties["id"]
			if !okS || !okT {
				continue
			}

			cypher := fmt.Sprintf(`
				MATCH (s:%s {id: $sourceId})
				MATCH (t:%s {id: $targetId})
				MERGE (s)-[r:%s]->(t)
				SET r += $props
			`, e.SourceNode.Label, e.TargetNode.Label, e.Type)

			params := map[string]interface{}{
				"sourceId": sID,
				"targetId": tID,
				"props":    e.Properties,
			}
			if params["props"] == nil {
				params["props"] = map[string]interface{}{}
			}
			
			_, err := tx.Run(ctx, cypher, params)
			if err != nil {
				return nil, err
			}
		}

		return nil, nil
	})

	return err
}
