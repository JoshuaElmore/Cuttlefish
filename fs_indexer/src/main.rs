use std::env;
use std::error::Error;
use std::fs::Metadata;
use std::os::unix::fs::MetadataExt;
use std::path::Path;
use std::sync::Arc;
use std::thread;
use ignore::WalkBuilder;
use csv::Writer;
use crossbeam_channel::unbounded;

struct FileRecord {
    path: String,
    inode: u64,
    size: u64,
    mode: u32,
    uid: u32,
    gid: u32,
    atime: i64,
    mtime: i64,
    ctime: i64,
}

fn main() -> Result<(), Box<dyn Error>> {
    let args: Vec<String> = env::args().collect();
    if args.len() < 2 {
        eprintln!("Usage: {} <path_to_scan> [threads]", args[0]);
        std::process::exit(1);
    }

    let scan_path = &args[1];
    let num_threads = if args.len() > 2 {
        args[2].parse::<usize>().unwrap_or(num_cpus::get())
    } else {
        num_cpus::get()
    };

    let output_file = "filesystem_index.csv";
    println!("Scanning: {} with {} threads...", scan_path, num_threads);

    // 1. Create a channel for communicating between workers and the writer
    let (tx, rx) = unbounded::<FileRecord>();

    // 2. Spawn the Writer Thread
    // We do this in a separate thread so workers don't block on Disk I/O for the CSV
    let writer_handle = thread::spawn(move || -> Result<(), Box<dyn Error + Send + Sync>> {
        let mut wtr = Writer::from_path(output_file)?;
        wtr.write_record(&["path", "inode", "size_bytes", "permissions", "uid", "gid", "atime", "mtime", "ctime"])?;

        for record in rx {
            wtr.write_record(&[
                record.path,
                record.inode.to_string(),
                record.size.to_string(),
                format!("{:o}", record.mode),
                record.uid.to_string(),
                record.gid.to_string(),
                record.atime.to_string(),
                record.mtime.to_string(),
                record.ctime.to_string(),
            ])?;
        }
        wtr.flush()?;
        Ok(())
    });

    // 3. Configure the Parallel Walker
    // WalkBuilder from the 'ignore' crate handles the multi-threaded recursion
    let walker = WalkBuilder::new(scan_path)
        .threads(num_threads)
        .standard_filters(false) // Don't ignore .gitignore files, index everything
        .build_parallel();

    // The ParallelWalker uses a closure that is executed on multiple threads
    walker.run(|| {
        let tx = tx.clone();
        Box::new(move |entry| {
            if let Ok(entry) = entry {
                if let Ok(meta) = entry.metadata() {
                    let record = FileRecord {
                        path: entry.path().to_string_lossy().into_owned(),
                        inode: meta.ino(),
                        size: meta.len(),
                        mode: meta.mode(),
                        uid: meta.uid(),
                        gid: meta.gid(),
                        atime: meta.atime(),
                        mtime: meta.mtime(),
                        ctime: meta.ctime(),
                    };
                    let _ = tx.send(record);
                }
            }
            ignore::WalkState::Continue
        })
    });

    // 4. Close the original sender so the writer thread knows when to stop
    drop(tx);

    // Wait for writer to finish
    writer_handle.join().expect("Writer thread panicked").expect("Writer error");

    println!("Indexing complete. Data saved to {}", output_file);
    Ok(())
}

// Add num_cpus as a helper for default thread count
mod num_cpus {
    pub fn get() -> usize {
        std::thread::available_parallelism().map(|n| n.get()).unwrap_or(4)
    }
}
