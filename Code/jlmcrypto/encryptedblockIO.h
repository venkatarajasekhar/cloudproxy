//
//  File: encryptedblockIO.h, encrypted IO, definitions
//
//  Copyright (c) 2012, John Manferdelli.  All rights reserved.
//      Some contributions (c) Intel Corporation
//
// Use, duplication and disclosure of this file and derived works of
// this file are subject to and licensed under the Apache License dated
// January, 2004, (the "License").  This License is contained in the
// top level directory originally provided with the CloudProxy Project.
// Your right to use or distribute this file, or derived works thereof,
// is subject to your being bound by those terms and your use indicates
// consent to those terms.
//
// If you distribute this file (or portions derived therefrom), you must
// include License in or with the file and, in the event you do not include
// the entire License in the file, the file must contain a reference
// to the location of the License.


#ifndef __ENCRYPTEDBLOCKIO_H
#define __ENCRYPTEDBLOCKIO_H


// Get rid of this
#define  BLOCKBUFSIZE  (1024+3*BIGBLOCKSIZE)


// --------------------------------------------------------------------



#include "jlmTypes.h"
#include "aes.h"
#include "sha256.h"
#include "modesandpadding.h"


class encryptedFileread {
public:
    bool    m_fFirstBlockRead;
    bool    m_fInitialized;

    u32     m_uAlg;
    u32     m_uMode;
    u32     m_uPad;
    u32     m_uHmac;

    int     m_fileSize;
    int     m_fileLeft;
    int     m_dataSize;

    bool    m_fFinalProcessed;

    int     m_iBlockSize;

    int     m_iBufIn;
    int     m_iBufOut;
    int     m_iInStart;
    int     m_iOutStart;

    byte    m_rguBufIn[BLOCKBUFSIZE];
    byte    m_rguBufOut[BLOCKBUFSIZE];

    u32     m_uCombinedAlgId;

    gcm     m_oGCM;
    cbc     m_oCBC;

    encryptedFileread();
    ~encryptedFileread();

    bool    AES128GCMDecryptBlocks(int iRead);
    bool    AES128CBCDecryptBlocks(int iRead);
    int     AES128GCMDecrypt(int iRead, int bufsize, byte* buf);
    int     AES128CBCDecrypt(int iRead, int bufsize, byte* buf);

    bool    initDec(int filesize, int datasize, byte* key, int keyBitSize, u32 alg=NOALG, u32 pad=NOPAD, 
                    u32 mode=NOMODE, u32 hmac=NOHMAC); 
    int     EncRead(int iRead, byte* buf, int size);
    bool    closeEnc();
};


class encryptedFilewrite {
public:
    bool    m_fFirstBlockWritten;
    bool    m_fFirstBlockRead;
    bool    m_fInitialized;

    u32     m_uAlg;
    u32     m_uMode;
    u32     m_uPad;
    u32     m_uHmac;

    int     m_fileLeft;
    int     m_fileSize;
    int     m_dataSize;

    int     m_iBlockSize;
    bool    m_fFinalProcessed;

    int     m_iBufIn;
    int     m_iBufOut;
    int     m_iInStart;
    int     m_iOutStart;

    byte    m_rguBufIn[BLOCKBUFSIZE];
    byte    m_rguBufOut[BLOCKBUFSIZE];

    u32     m_uCombinedAlgId;

    gcm     m_oGCM;
    cbc     m_oCBC;

    encryptedFilewrite();
    ~encryptedFilewrite();

    int     AES128GCMEncrypt(int iWrite, int bufsize, byte* buf);
    int     AES128CBCEncrypt(int iWrite, int bufsize, byte* buf);
    bool    AES128CBCEncryptBlocks(int iWrite);
    bool    AES128GCMEncryptBlocks(int iWrite);

    bool    initEnc(int datasize, int filesize, byte* key, int keyBitSize, u32 alg=NOALG, u32 pad=NOPAD, 
                    u32 mode=NOMODE, u32 hmac=NOHMAC); 
    int     EncWrite(int file, byte* buf, int size);
};


#endif


// --------------------------------------------------------------------


