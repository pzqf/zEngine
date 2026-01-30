
#!/usr/bin/env python3
import xml.etree.ElementTree as ET
import json
import os
import argparse
import sys

def set_utf8_encoding():
    """设置标准输出为UTF-8编码"""
    if sys.platform == 'win32':
        import codecs
        sys.stdout = codecs.getwriter('utf-8')(sys.stdout.buffer)
        sys.stderr = codecs.getwriter('utf-8')(sys.stderr.buffer)

def parse_graphml(filename):
    tree = ET.parse(filename)
    root = tree.getroot()
    
    namespaces = {
        'xmlns': 'http://graphml.graphdrawing.org/xmlns',
        'y': 'http://www.yworks.com/xml/graphml'
    }
    
    graph = root.find('xmlns:graph', namespaces)
    
    nodes = []
    edges = []
    
    # Parse nodes
    for node in graph.findall('xmlns:node', namespaces):
        node_id = node.get('id')
        
        data = node.find('xmlns:data', namespaces)
        if data is None:
            continue
            
        shape_node = data.find('y:ShapeNode', namespaces)
        if shape_node is None:
            continue
            
        node_label = shape_node.find('y:NodeLabel', namespaces)
        if node_label is None or node_label.text is None:
            continue
            
        content = node_label.text.strip()
        
        nodes.append({
            'id': node_id,
            'content': content
        })
    
    # Parse edges
    for edge in graph.findall('xmlns:edge', namespaces):
        edge_id = edge.get('id')
        source = edge.get('source')
        target = edge.get('target')
        
        data = edge.find('xmlns:data', namespaces)
        if data is None:
            continue
            
        polyline_edge = data.find('y:PolyLineEdge', namespaces)
        if polyline_edge is None:
            continue
            
        edge_label = polyline_edge.find('y:EdgeLabel', namespaces)
        content = ''
        if edge_label is not None and edge_label.text is not None:
            content = edge_label.text.strip()
        
        edges.append({
            'id': edge_id,
            'source': source,
            'target': target,
            'content': content
        })
    
    return {
        'nodes': nodes,
        'edges': edges
    }

def save_json(data, output_file):
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(data, f, indent=2, ensure_ascii=False)

def convert_single_file(input_file, output_file):
    print(f"Converting {input_file} to {output_file}...")
    
    try:
        data = parse_graphml(input_file)
        save_json(data, output_file)
        print(f"✓ Successfully converted {input_file}")
        
        # Print stats
        print(f"  - Nodes: {len(data['nodes'])}")
        print(f"  - Edges: {len(data['edges'])}")
        
    except Exception as e:
        print(f"✗ Error converting {input_file}: {str(e)}")
        return False
    
    return True

def convert_directory(input_dir, output_dir):
    if not os.path.exists(output_dir):
        os.makedirs(output_dir)
    
    success_count = 0
    total_count = 0
    
    for filename in os.listdir(input_dir):
        if filename.endswith('.graphml'):
            total_count += 1
            input_file = os.path.join(input_dir, filename)
            output_file = os.path.join(output_dir, filename.replace('.graphml', '.json'))
            
            if convert_single_file(input_file, output_file):
                success_count += 1
    
    print(f"\nConversion complete: {success_count}/{total_count} files converted successfully")

def main():
    set_utf8_encoding()
    parser = argparse.ArgumentParser(description='Convert GraphML files to JSON format')
    parser.add_argument('input', help='Input GraphML file or directory')
    parser.add_argument('-o', '--output', help='Output JSON file or directory')
    
    args = parser.parse_args()
    
    input_path = args.input
    
    if args.output:
        output_path = args.output
    else:
        if os.path.isfile(input_path):
            output_path = input_path.replace('.graphml', '.json')
        else:
            output_path = input_path
    
    if os.path.isfile(input_path):
        convert_single_file(input_path, output_path)
    elif os.path.isdir(input_path):
        convert_directory(input_path, output_path)
    else:
        print(f"Error: {input_path} does not exist")
        return 1
    
    return 0

if __name__ == '__main__':
    exit(main())

