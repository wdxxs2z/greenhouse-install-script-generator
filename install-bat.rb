#!/usr/bin/env ruby
require 'open-uri'
require 'openssl'
require 'json'
require 'yaml'
require 'fileutils'

def getJSON(path)
  url = "https://192.168.50.4:25555#{path}"
  JSON.parse(open(url, :ssl_verify_mode => OpenSSL::SSL::VERIFY_NONE, :http_basic_authentication=>["admin", "admin"]).read)
end

deployments = getJSON("/deployments")
deployment = deployments.detect do |d|
  (%w(cf diego) - d["releases"].map{|r|r["name"]}).empty?
end["name"]

manifest = YAML.load(getJSON("/deployments/#{deployment}")["manifest"]).to_hash
consul_ips = manifest["properties"]["consul"]["agent"]["servers"]["lan"].join(',')
# etcd_cluster="https://etcd.service.consul:4001"
cf_etcd_cluster = manifest["properties"]["etcd"]["machines"].map{|ip|"http://#{ip}:4001"}.first

zones = manifest["jobs"].map do |job|
  job["properties"]["diego"]["rep"]["zone"] rescue nil
end.select{|a|a}.uniq
loggregator_shared_secret=manifest["properties"]["loggregator_endpoint"]["shared_secret"]

puts "Generate files in output directory"
FileUtils.mkdir_p "output"

certs = manifest["properties"]["diego"]["etcd"]
puts "  output/ca.crt"
open("output/ca.crt", 'w') do |f|
  f.write certs["ca_cert"]
end
puts "  output/client.crt"
open("output/client.crt", 'w') do |f|
  f.write certs["client_cert"]
end
puts "  output/client.key"
open("output/client.key", 'w') do |f|
  f.write certs["client_key"]
end

etcd_ca_file='%cd%\ca.crt'

zones.each do |zone|
  puts "  output/install_#{zone}.bat"
  open("output/install_#{zone}.bat", 'w') do |f|
    f.write <<-BAT
msiexec /norestart /i diego.msi
  ADMIN_USERNAME=[USERNAME]
  ADMIN_PASSWORD=[PASSWORD]
  CONSUL_IPS=#{consul_ips}
  CF_ETCD_CLUSTER=#{cf_etcd_cluster}
  STACK=windows2012R2
  REDUNDANCY_ZONE=#{zone}
  LOGGREGATOR_SHARED_SECRET=#{loggregator_shared_secret}
  ETCD_CA_FILE=%cd%\\ca.crt
  ETCD_CERT_FILE=%cd%\\client.crt
  ETCD_KEY_FILE=%cd%\\client.key
BAT
  end
end
