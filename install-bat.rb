#!/usr/bin/env ruby
require 'open-uri'
require 'openssl'
require 'json'
require 'yaml'
require 'fileutils'

BOSH_URL = ARGV[0]
OUTPUT_DIR = ARGV[1]

def getJSON(path)
  url = BOSH_URL + path
  JSON.parse(open(url, :ssl_verify_mode => OpenSSL::SSL::VERIFY_NONE, :http_basic_authentication=>["admin", "admin"]).read)
end

deployments = getJSON("/deployments")
deployment = deployments.detect do |d|
  (%w(cf diego) - d["releases"].map{|r|r["name"]}).empty?
end["name"]

manifest = getJSON("/deployments/#{deployment}")["manifest"]
manifest = YAML.load(manifest).to_hash
consul_ips = manifest["properties"]["consul"]["agent"]["servers"]["lan"].join(',')
# etcd_cluster="https://etcd.service.consul:4001"
cf_etcd_cluster = manifest["properties"]["etcd"]["machines"].map{|ip|"http://#{ip}:4001"}.first

zones = manifest["jobs"].map do |job|
  job["properties"]["diego"]["rep"]["zone"] rescue nil
end.select{|a|a}.uniq
loggregator_shared_secret=manifest["properties"]["loggregator_endpoint"]["shared_secret"]

puts "Generate files in output directory"
FileUtils.mkdir_p OUTPUT_DIR

certs = manifest["properties"]["diego"]["etcd"]
puts "  #{OUTPUT_DIR}/ca.crt"
open("#{OUTPUT_DIR}/ca.crt", 'w') do |f|
  f.write certs["ca_cert"]
end
puts "  #{OUTPUT_DIR}/client.crt"
open("#{OUTPUT_DIR}/client.crt", 'w') do |f|
  f.write certs["client_cert"]
end
puts "  #{OUTPUT_DIR}/client.key"
open("#{OUTPUT_DIR}/client.key", 'w') do |f|
  f.write certs["client_key"]
end

zones.each do |zone|
  puts "  #{OUTPUT_DIR}/install_#{zone}.bat"
  open("#{OUTPUT_DIR}/install_#{zone}.bat", 'w') do |f|
    f.write <<-BAT
msiexec /norestart /i diego.msi ^\r
  ADMIN_USERNAME=[USERNAME] ^\r
  ADMIN_PASSWORD=[PASSWORD] ^\r
  CONSUL_IPS=#{consul_ips} ^\r
  CF_ETCD_CLUSTER=#{cf_etcd_cluster} ^\r
  STACK=windows2012R2 ^\r
  REDUNDANCY_ZONE=#{zone} ^\r
  LOGGREGATOR_SHARED_SECRET=#{loggregator_shared_secret} ^\r
  ETCD_CA_FILE=%cd%\\ca.crt ^\r
  ETCD_CERT_FILE=%cd%\\client.crt ^\r
  ETCD_KEY_FILE=%cd%\\client.key \r
BAT
  end
end
