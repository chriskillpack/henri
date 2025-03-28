#!/usr/bin/env python3

# This is a one-time script to populate the image_width and height columns of the
# image table, as data backfill. This script was written by Claude.AI.

import os
import sqlite3
from datetime import datetime
from PIL import Image

def update_image_dimensions(db_path):
    """
    Update the image_width and image_height columns for all rows in the images table
    where these values are NULL or 0.
    
    Args:
        db_path (str): Path to the SQLite database file
    """
    # Connect to the database
    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()
    
    # Get all rows where dimensions are missing or zero
    cursor.execute("""
        SELECT id, image_path FROM images 
        WHERE image_width IS NULL OR image_width = 0 
        OR image_height IS NULL OR image_height = 0
    """)
    rows = cursor.fetchall()
    
    print(f"Found {len(rows)} images with missing dimensions.")
    if not rows:
        print("No images to update. Exiting.")
        conn.close()
        return
    
    # Process each image
    updated_count = 0
    error_count = 0
    
    for row_id, image_path in rows:
        try:
            # Check if file exists
            if not os.path.exists(image_path):
                print(f"Warning: Image not found at path: {image_path}")
                continue
                
            # Open the image and get dimensions
            with Image.open(image_path) as img:
                width, height = img.size
            
            # Update the database
            cursor.execute("""
                UPDATE images
                SET image_width = ?, image_height = ?
                WHERE id = ?
            """, (width, height, row_id))
            
            updated_count += 1
            print(f"Updated image {row_id}: {image_path} ({width}x{height})")
            
        except Exception as e:
            print(f"Error processing image {row_id} at {image_path}: {str(e)}")
            error_count += 1
    
    # Commit changes
    conn.commit()
    conn.close()
    
    # Print summary
    print(f"\nUpdate complete:")
    print(f"  - Total images processed: {len(rows)}")
    print(f"  - Successfully updated: {updated_count}")
    print(f"  - Errors encountered: {error_count}")

if __name__ == "__main__":
    import argparse
    
    parser = argparse.ArgumentParser(description="Update missing image dimensions in SQLite database")
    parser.add_argument("db_path", help="Path to the SQLite database file")
    
    args = parser.parse_args()
    
    print(f"Starting update process on database: {args.db_path}")
    update_image_dimensions(args.db_path)