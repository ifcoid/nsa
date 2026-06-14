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

// PaperRelation represents a relation from a Paper node to a connected entity
// (Method, Dataset, Finding, etc.) as stored in the knowledge graph.
type PaperRelation struct {
	RelationType string
	TargetLabel  string
	TargetName   string
	Properties   map[string]interface{}
}

// ClaimEvidence represents evidence found in the knowledge graph related to a claim.
type ClaimEvidence struct {
	DOI              string
	RelationType     string
	MatchedNodeLabel string
	MatchedNodeName  string
}

// QueryPaperRelations returns the relations (paper->method, paper->dataset, paper->finding edges)
// for a given paper DOI within a session.
func (r *Neo4jRepository) QueryPaperRelations(ctx context.Context, sessionID, doi string) ([]PaperRelation, error) {
	if r == nil || r.driver == nil {
		return nil, nil
	}
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		cypher := `
			MATCH (p:Paper)-[r]->(t)
			WHERE p.session_id = $sessionID AND p.doi = $doi
			RETURN type(r) AS relType, labels(t)[0] AS targetLabel,
			       coalesce(t.name, t.id, '') AS targetName, properties(r) AS props
		`
		params := map[string]interface{}{
			"doi":       doi,
			"sessionID": sessionID,
		}
		records, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var relations []PaperRelation
		for records.Next(ctx) {
			rec := records.Record()
			relType, _ := rec.Get("relType")
			targetLabel, _ := rec.Get("targetLabel")
			targetName, _ := rec.Get("targetName")
			props, _ := rec.Get("props")

			pr := PaperRelation{
				RelationType: fmt.Sprintf("%v", relType),
				TargetLabel:  fmt.Sprintf("%v", targetLabel),
				TargetName:   fmt.Sprintf("%v", targetName),
			}
			if propMap, ok := props.(map[string]interface{}); ok {
				pr.Properties = propMap
			}
			relations = append(relations, pr)
		}
		return relations, records.Err()
	})
	if err != nil {
		return nil, err
	}
	if relations, ok := result.([]PaperRelation); ok {
		return relations, nil
	}
	return nil, nil
}

// QueryClaimEvidence searches nodes/edges matching a claim string and returns
// related paper DOIs and relationship types.
func (r *Neo4jRepository) QueryClaimEvidence(ctx context.Context, claim string) ([]ClaimEvidence, error) {
	if r == nil || r.driver == nil {
		return nil, nil
	}
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// Search for nodes whose name/id contains part of the claim text,
		// then return the connected Paper DOIs.
		cypher := `
			MATCH (p:Paper)-[r]->(t)
			WHERE toLower(t.name) CONTAINS toLower($claim)
			   OR toLower(t.id) CONTAINS toLower($claim)
			RETURN DISTINCT p.doi AS doi, type(r) AS relType,
			       labels(t)[0] AS matchedLabel, coalesce(t.name, t.id, '') AS matchedName
			LIMIT 50
		`
		params := map[string]interface{}{
			"claim": claim,
		}
		records, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var evidence []ClaimEvidence
		for records.Next(ctx) {
			rec := records.Record()
			doi, _ := rec.Get("doi")
			relType, _ := rec.Get("relType")
			matchedLabel, _ := rec.Get("matchedLabel")
			matchedName, _ := rec.Get("matchedName")

			ce := ClaimEvidence{
				DOI:              fmt.Sprintf("%v", doi),
				RelationType:     fmt.Sprintf("%v", relType),
				MatchedNodeLabel: fmt.Sprintf("%v", matchedLabel),
				MatchedNodeName:  fmt.Sprintf("%v", matchedName),
			}
			evidence = append(evidence, ce)
		}
		return evidence, records.Err()
	})
	if err != nil {
		return nil, err
	}
	if evidence, ok := result.([]ClaimEvidence); ok {
		return evidence, nil
	}
	return nil, nil
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
