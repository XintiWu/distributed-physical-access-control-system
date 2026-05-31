#!/usr/bin/env python3
import os
import re
import sys
import subprocess

def load_env(env_path):
    env = {}
    if not os.path.exists(env_path):
        return env
    with open(env_path, 'r') as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith('#'):
                continue
            if '=' in line:
                k, v = line.split('=', 1)
                k = k.strip()
                v = v.strip().strip('"').strip("'")
                env[k] = v
    return env

def substitute_env_vars(content, env):
    def replace(match):
        var_name = match.group(1) or match.group(2)
        val = env.get(var_name, os.environ.get(var_name))
        if val is None:
            return match.group(0)
        return val

    pattern = re.compile(r'\${([A-Za-z0-9_]+)}|\$([A-Za-z0-9_]+)')
    return pattern.sub(replace, content)

def main():
    if len(sys.argv) < 2:
        print("Usage: apply-k8s.py <yaml-file>")
        sys.exit(1)
        
    yaml_file = sys.argv[1]
    project_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    env_path = os.path.join(project_root, '.env')
    
    env = load_env(env_path)
    
    # Auto-extract host from CLICKHOUSE_ADDR (remove port if exists)
    if 'CLICKHOUSE_ADDR' in env:
        env['CLICKHOUSE_HOST'] = env['CLICKHOUSE_ADDR'].split(':')[0]
    
    with open(yaml_file, 'r') as f:
        content = f.read()
        
    substituted = substitute_env_vars(content, env)
    
    proc = subprocess.Popen(['kubectl', 'apply', '-f', '-'], stdin=subprocess.PIPE, text=True)
    proc.communicate(input=substituted)
    sys.exit(proc.returncode)

if __name__ == '__main__':
    main()
